package dto

type SeedanceChannelOption struct {
	Id     int    `json:"id"`
	Name   string `json:"name"`
	Status int    `json:"status"`
}

type UpdateUserSeedanceChannelsRequest struct {
	SeedanceInnerChannelID *int `json:"seedance_inner_channel_id,omitempty" binding:"omitempty,gte=0"`
	SeedanceOuterChannelID *int `json:"seedance_outer_channel_id,omitempty" binding:"omitempty,gte=0"`
}

type UserSeedanceChannels struct {
	SeedanceInnerChannelID int `json:"seedance_inner_channel_id"`
	SeedanceOuterChannelID int `json:"seedance_outer_channel_id"`
}

type BaseAssetRequest struct {
	ProjectName *string `json:"ProjectName" binding:"omitempty,oneof=default"`
}

type CreateAssetGroupRequest struct {
	BaseAssetRequest
	GroupType   string  `json:"GroupType" binding:"required,oneof=AIGC"`
	Name        string  `json:"Name" binding:"required"`
	Description *string `json:"Description"`
}

type CreateAssetRequest struct {
	BaseAssetRequest
	AssetType  string           `json:"AssetType" binding:"required,oneof=Image Video Audio"`
	GroupId    string           `json:"GroupId" binding:"required"`
	URL        string           `json:"URL" binding:"required"`
	Name       string           `json:"Name" binding:"required"`
	Moderation *AssetModeration `json:"Moderation" binding:"omitempty"`
}

type AssetModeration struct {
	Strategy string `json:"Strategy" binding:"required,oneof=Skip"`
}

type ListAssetGroupsFilter struct {
	GroupIds  []string `json:"GroupIds"`
	Name      *string  `json:"Name"`
	GroupType *string  `json:"GroupType" binding:"omitempty,oneof=AIGC"`
}

type ListAssetGroupsRequest struct {
	BaseAssetRequest
	Filter     *ListAssetGroupsFilter `json:"Filter"`
	PageNumber int                    `json:"PageNumber"`
	PageSize   int                    `json:"PageSize"`
	SortBy     string                 `json:"SortBy"`
	SortOrder  string                 `json:"SortOrder"`
}

type ListAssetsFilter struct {
	GroupIds  []string `json:"GroupIds"`
	GroupType *string  `json:"GroupType" binding:"omitempty,oneof=AIGC"`
	Statuses  []string `json:"Statuses"`
	Name      *string  `json:"Name"`
}

type ListAssetsRequest struct {
	BaseAssetRequest
	Filter     *ListAssetsFilter `json:"Filter"`
	PageNumber int               `json:"PageNumber"`
	PageSize   int               `json:"PageSize"`
	SortBy     string            `json:"SortBy"`
	SortOrder  string            `json:"SortOrder"`
}

type GetAssetRequest struct {
	BaseAssetRequest
	Id string `json:"Id" binding:"required"`
}

type GetAssetGroupRequest struct {
	BaseAssetRequest
	Id string `json:"Id" binding:"required"`
}

type UpdateAssetGroupRequest struct {
	BaseAssetRequest
	Id          string  `json:"Id" binding:"required"`
	Name        string  `json:"Name" binding:"required"`
	Description *string `json:"Description"`
}

type UpdateAssetRequest struct {
	BaseAssetRequest
	Id   string `json:"Id" binding:"required"`
	Name string `json:"Name" binding:"required"`
}

type DeleteAssetRequest struct {
	BaseAssetRequest
	Id string `json:"Id" binding:"required"`
}

type DeleteAssetGroupRequest struct {
	BaseAssetRequest
	Id string `json:"Id" binding:"required"`
}

type CreateVisualValidateSessionRequest struct {
	BaseAssetRequest
	CallbackURL string `json:"CallbackUrl" binding:"required"`
}

type GetVisualValidateResultRequest struct {
	BaseAssetRequest
	BytedToken string `json:"BytedToken" binding:"required"`
}

type AssetBaseResult struct {
	ResponseMetadata struct {
		RequestId string `json:"RequestId"`
		Action    string `json:"Action"`
		Version   string `json:"Version"`
		Service   string `json:"Service"`
		Region    string `json:"Region"`
	} `json:"ResponseMetadata"`
	Result any `json:"Result"`
}

type CreateAssetGroupResponse struct {
	Id string `json:"Id"`
}

type CreateAssetResponse struct {
	Id string `json:"Id"`
}

type ListAssetGroupsResponse struct {
	Items      []GetAssetGroupResponse `json:"Items"`
	TotalCount int64                   `json:"TotalCount"`
	PageNumber int                     `json:"PageNumber"`
	PageSize   int                     `json:"PageSize"`
}

type ListAssetsResponse struct {
	Items      []GetAssetResponse `json:"Items"`
	TotalCount int64              `json:"TotalCount"`
	PageNumber int                `json:"PageNumber"`
	PageSize   int                `json:"PageSize"`
}

type GetAssetResponse struct {
	Id          string      `json:"Id"`
	GroupId     string      `json:"GroupId"`
	URL         string      `json:"URL"`
	Name        string      `json:"Name"`
	AssetType   string      `json:"AssetType"`
	ProjectName string      `json:"ProjectName"`
	Status      string      `json:"Status"`
	CreateTime  string      `json:"CreateTime"`
	UpdateTime  string      `json:"UpdateTime"`
	Error       *AssetError `json:"Error,omitempty"`
}

type AssetError struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
}

type GetAssetGroupResponse struct {
	Id          string `json:"Id"`
	Name        string `json:"Name"`
	Description string `json:"Description"`
	GroupType   string `json:"GroupType"`
	ProjectName string `json:"ProjectName"`
	CreateTime  string `json:"CreateTime"`
	UpdateTime  string `json:"UpdateTime"`
}

type UpdateAssetGroupResponse struct {
	Id string `json:"Id"`
}

type UpdateAssetResponse struct {
	Id string `json:"Id"`
}

type DeleteAssetResponse struct{}

type DeleteAssetGroupResponse struct{}

type CreateVisualValidateSessionResponse struct {
	BytedToken  string `json:"BytedToken"`
	H5Link      string `json:"H5Link"`
	CallbackURL string `json:"CallbackURL"`
}

type GetVisualValidateResultResponse struct {
	GroupId string `json:"GroupId"`
}
