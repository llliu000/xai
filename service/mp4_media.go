package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/abema/go-mp4"
)

type MP4Media = model.VideoMediaResult

var ErrUnsupportedMP4 = errors.New("unsupported MP4 file")

func MP4MediaByteLimit() int64 {
	if constant.MaxFileDownloadMB > 0 {
		return int64(min(constant.MaxFileDownloadMB, 512)) << 20
	}
	return 64 << 20
}

// ProbePublicMP4 inspects the same public URL that will be submitted or shown
// as a result. The file is bounded and removed after inspection.
func ProbePublicMP4(ctx context.Context, rawURL string) (MP4Media, string, error) {
	resp, err := openPublicVideoMedia(ctx, rawURL, false)
	if err != nil {
		return MP4Media{}, "", err
	}
	defer resp.Body.Close()
	limit := MP4MediaByteLimit()
	if resp.ContentLength > limit {
		return MP4Media{}, "", ErrUnsupportedMP4
	}
	file, err := os.CreateTemp("", "xai-video-*.mp4")
	if err != nil {
		return MP4Media{}, "", err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return MP4Media{}, "", err
	}
	if size == 0 || size > limit {
		return MP4Media{}, "", ErrUnsupportedMP4
	}
	media, err := probeMP4(file)
	if err != nil {
		return MP4Media{}, "", err
	}
	media.Size, media.SHA256 = size, hex.EncodeToString(hash.Sum(nil))
	return media, resp.Request.URL.String(), nil
}

func probeMP4(r io.ReadSeeker) (MP4Media, error) {
	var media MP4Media
	boxes, err := mp4.ExtractBoxes(r, nil, []mp4.BoxPath{{mp4.BoxTypeFtyp()}, {mp4.BoxTypeMoov()}, {mp4.BoxTypeMoof()}, {mp4.BoxTypeMdat()}})
	if err != nil || len(boxes) < 3 || boxes[0].Type != mp4.BoxTypeFtyp() {
		return media, ErrUnsupportedMP4
	}
	var moov *mp4.BoxInfo
	var hasData bool
	for _, box := range boxes[1:] {
		switch box.Type {
		case mp4.BoxTypeMoov():
			if moov != nil || box.Size > 4<<20 {
				return media, ErrUnsupportedMP4
			}
			moov = box
		case mp4.BoxTypeMdat():
			hasData = hasData || box.Size > box.HeaderSize
		default:
			return media, ErrUnsupportedMP4
		}
	}
	if moov == nil || !hasData {
		return media, ErrUnsupportedMP4
	}
	tracks, err := mp4.ExtractBox(r, moov, mp4.BoxPath{mp4.BoxTypeTrak()})
	if err != nil || len(tracks) == 0 || len(tracks) > 32 {
		return media, ErrUnsupportedMP4
	}
	for _, track := range tracks {
		fields, err := mp4.ExtractBoxesWithPayload(r, track, []mp4.BoxPath{
			{mp4.BoxTypeTkhd()},
			{mp4.BoxTypeMdia(), mp4.BoxTypeMdhd()},
			{mp4.BoxTypeMdia(), mp4.BoxTypeHdlr()},
		})
		if err != nil {
			return media, ErrUnsupportedMP4
		}
		var handler string
		var trackHeader *mp4.Tkhd
		var mediaHeader *mp4.Mdhd
		for _, field := range fields {
			switch payload := field.Payload.(type) {
			case *mp4.Tkhd:
				trackHeader = payload
			case *mp4.Mdhd:
				mediaHeader = payload
			case *mp4.Hdlr:
				handler = string(payload.HandlerType[:])
			}
		}
		if handler == "soun" {
			media.HasAudio = true
			continue
		}
		if handler != "vide" {
			continue
		}
		if media.Width != 0 || trackHeader == nil || mediaHeader == nil || mediaHeader.Timescale == 0 ||
			trackHeader.Width&0xffff != 0 || trackHeader.Height&0xffff != 0 {
			return media, ErrUnsupportedMP4
		}
		media.Width, media.Height = int(trackHeader.GetWidthInt()), int(trackHeader.GetHeightInt())
		matrix := trackHeader.Matrix
		switch {
		case matrix[0] == 0 && (matrix[1] == 1<<16 || matrix[1] == -1<<16) && matrix[3] == -matrix[1] && matrix[4] == 0:
			media.Width, media.Height = media.Height, media.Width
		case (matrix[0] == 1<<16 || matrix[0] == -1<<16) && matrix[4] == matrix[0] && matrix[1] == 0 && matrix[3] == 0:
		default:
			return media, ErrUnsupportedMP4
		}
		entries, err := mp4.ExtractBoxesWithPayload(r, track, []mp4.BoxPath{
			{mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStts()},
		})
		if err != nil || len(entries) != 1 {
			return media, ErrUnsupportedMP4
		}
		var samples, ticks float64
		for _, entry := range entries[0].Payload.(*mp4.Stts).Entries {
			samples += float64(entry.SampleCount)
			ticks += float64(entry.SampleCount) * float64(entry.SampleDelta)
		}
		if ticks == 0 {
			return media, ErrUnsupportedMP4
		}
		media.Duration = ticks / float64(mediaHeader.Timescale)
		media.FPS = samples / media.Duration
		codecs, err := mp4.ExtractBoxes(r, track, []mp4.BoxPath{
			{mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStsd(), mp4.StrToBoxType("avc1")},
			{mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStsd(), mp4.StrToBoxType("hvc1")},
			{mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStsd(), mp4.StrToBoxType("hev1")},
		})
		if err != nil || len(codecs) != 1 {
			return media, ErrUnsupportedMP4
		}
		media.Codec = "h265"
		if codecs[0].Type == mp4.StrToBoxType("avc1") {
			media.Codec = "h264"
		}
	}
	if media.Width == 0 || media.Height == 0 {
		return media, ErrUnsupportedMP4
	}
	return media, nil
}
