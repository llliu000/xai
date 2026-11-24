package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/abema/go-mp4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mp4TestTransport func(*http.Request) (*http.Response, error)

func (f mp4TestTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestProbePublicMP4ReportsMeasuredFileFacts(t *testing.T) {
	box := func(kind string, contents ...[]byte) []byte {
		size := 8
		for _, content := range contents {
			size += len(content)
		}
		data := binary.BigEndian.AppendUint32(nil, uint32(size))
		data = append(data, kind...)
		for _, content := range contents {
			data = append(data, content...)
		}
		return data
	}
	payload := func(value mp4.IBox) []byte {
		var data bytes.Buffer
		_, err := mp4.Marshal(&data, value, mp4.Context{})
		require.NoError(t, err)
		return data.Bytes()
	}
	fixture := bytes.Join([][]byte{
		box("ftyp", []byte("isom\x00\x00\x00\x00isom")),
		box("mdat", []byte{1, 2, 3}),
		box("moov",
			box("trak", box("tkhd", payload(&mp4.Tkhd{TrackID: 1, Width: 1280 << 16, Height: 720 << 16, Matrix: [9]int32{1 << 16, 0, 0, 0, 1 << 16, 0, 0, 0, 1 << 30}})),
				box("mdia", box("mdhd", payload(&mp4.Mdhd{Timescale: 2400})), box("hdlr", payload(&mp4.Hdlr{HandlerType: [4]byte{'v', 'i', 'd', 'e'}})),
					box("minf", box("stbl",
						box("stts", payload(&mp4.Stts{EntryCount: 1, Entries: []mp4.SttsEntry{{SampleCount: 48, SampleDelta: 100}}})),
						box("stsd", payload(&mp4.Stsd{EntryCount: 1}), box("avc1", make([]byte, 78))),
					)),
				),
			),
			box("trak", box("mdia", box("hdlr", payload(&mp4.Hdlr{HandlerType: [4]byte{'s', 'o', 'u', 'n'}})))),
		),
	}, nil)
	previous := publicVideoMediaClient
	client := *previous
	client.Transport = mp4TestTransport(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(fixture)), ContentLength: int64(len(fixture)), Request: req}, nil
	})
	publicVideoMediaClient = &client
	t.Cleanup(func() { publicVideoMediaClient = previous })
	const source = "https://media.example.org/source.mp4"
	media, finalURL, err := ProbePublicMP4(context.Background(), source)
	require.NoError(t, err)
	assert.Equal(t, source, finalURL)
	assert.Equal(t, MP4Media{Width: 1280, Height: 720, Duration: 2, FPS: 24, Codec: "h264", HasAudio: true,
		Size: int64(len(fixture)), SHA256: fmt.Sprintf("%x", sha256.Sum256(fixture))}, media)

	client.Transport = mp4TestTransport(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusFound, Body: http.NoBody, Request: req, Header: http.Header{"Location": {"https://127.0.0.1/private.mp4"}}}, nil
	})
	_, _, err = ProbePublicMP4(context.Background(), source)
	require.Error(t, err)
}
