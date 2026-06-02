package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strconv"
)

type InstagramStalkOptions struct {
	IncludePosts           *bool
	IncludeReels           *bool
	IncludeAbout           *bool
	MaxItems               *int
	MaxPages               *int
	DelayMs                *int
	JitterMs               *int
	MaxRetries             *int
	BackoffBaseMs          *int
	BackoffMaxMs           *int
	BackoffJitterMs        *int
	TimeoutMs              *int
	WarmupDelayMs          *int
	WarmupJitterMs         *int
	ProfileOnlyOnRateLimit *bool
	Headful                *bool
}

type InstagramStalkResult struct {
	Status         string                    `json:"status"`
	Include        InstagramStalkInclude     `json:"include"`
	Auth           InstagramStalkAuth        `json:"auth"`
	TargetUsername string                    `json:"target_username"`
	Profile        InstagramStalkProfile     `json:"profile"`
	Warnings       []string                  `json:"warnings"`
	AboutAccount   *InstagramStalkAbout      `json:"about_account,omitempty"`
	PostsCount     int                       `json:"posts_count,omitempty"`
	Posts          []InstagramStalkMediaItem `json:"posts,omitempty"`
	ReelsCount     int                       `json:"reels_count,omitempty"`
	Reels          []InstagramStalkMediaItem `json:"reels,omitempty"`
	AllPostsCount  int                       `json:"all_posts_count,omitempty"`
}

type InstagramStalkInclude struct {
	AboutAccount bool `json:"about_account"`
	Posts        bool `json:"posts"`
	Reels        bool `json:"reels"`
}

type InstagramStalkAuth struct {
	Method                  string `json:"method"`
	ViewerUsername          string `json:"viewer_username"`
	HasSessionID            bool   `json:"has_sessionid"`
	CookieFileLoaded        string `json:"cookie_file_loaded"`
	CookieFileImportedCount int    `json:"cookie_file_imported_count"`
}

type InstagramStalkProfile struct {
	ID                string `json:"id"`
	Username          string `json:"username"`
	FullName          string `json:"full_name"`
	Biography         string `json:"biography"`
	ExternalURL       string `json:"external_url"`
	IsPrivate         bool   `json:"is_private"`
	IsVerified        bool   `json:"is_verified"`
	IsBusinessAccount bool   `json:"is_business_account"`
	FollowersCount    int64  `json:"followers_count"`
	FollowingCount    int64  `json:"following_count"`
	PostsCount        int64  `json:"posts_count"`
	ProfilePicURL     string `json:"profile_pic_url"`
}

type InstagramStalkAbout struct {
	Available                       bool                      `json:"available"`
	DateJoined                      string                    `json:"date_joined"`
	AccountBasedIn                  string                    `json:"account_based_in"`
	SharedFollowersCount            *int64                    `json:"shared_followers_count"`
	VerifiedSince                   string                    `json:"verified_since"`
	IsVerified                      bool                      `json:"is_verified"`
	ShowAccountTransparencyDetails  bool                      `json:"show_account_transparency_details"`
	TransparencyProductEnabled      bool                      `json:"transparency_product_enabled"`
	TransparencyLabel               string                    `json:"transparency_label"`
	MutualFollowersCount            *int64                    `json:"mutual_followers_count"`
	ProfileContextMutualFollowerIDs []string                  `json:"profile_context_mutual_follow_ids"`
	Source                          InstagramStalkAboutSource `json:"source"`
}

type InstagramStalkAboutSource struct {
	UsersInfoEndpoint bool `json:"users_info_endpoint"`
	AboutDialogScrape bool `json:"about_dialog_scrape"`
}

type InstagramStalkMediaItem struct {
	ID             string                       `json:"id"`
	PK             string                       `json:"pk"`
	Shortcode      string                       `json:"shortcode"`
	Permalink      string                       `json:"permalink"`
	ProductType    string                       `json:"product_type"`
	MediaType      int                          `json:"media_type"`
	IsVideo        bool                         `json:"is_video"`
	IsReel         bool                         `json:"is_reel"`
	TakenAtUTC     string                       `json:"taken_at_utc"`
	LikeCount      int64                        `json:"like_count"`
	CommentCount   int64                        `json:"comment_count"`
	PlayCount      *int64                       `json:"play_count"`
	ViewCount      *int64                       `json:"view_count"`
	Caption        string                       `json:"caption"`
	CaptionPreview string                       `json:"caption_preview"`
	ThumbnailURL   string                       `json:"thumbnail_url"`
	VideoURL       string                       `json:"video_url"`
	CarouselMedia  []InstagramStalkCarouselItem `json:"carousel_media"`
}

type InstagramStalkCarouselItem struct {
	ID        string `json:"id"`
	MediaType int    `json:"media_type"`
	IsVideo   bool   `json:"is_video"`
	ImageURL  string `json:"image_url"`
	VideoURL  string `json:"video_url"`
}

func (o InstagramStalkOptions) apply(q neturl.Values) {
	setBoolQuery(q, "includePosts", o.IncludePosts)
	setBoolQuery(q, "includeReels", o.IncludeReels)
	setBoolQuery(q, "includeAbout", o.IncludeAbout)
	setIntQuery(q, "maxItems", o.MaxItems)
	setIntQuery(q, "maxPages", o.MaxPages)
	setIntQuery(q, "delayMs", o.DelayMs)
	setIntQuery(q, "jitterMs", o.JitterMs)
	setIntQuery(q, "maxRetries", o.MaxRetries)
	setIntQuery(q, "backoffBaseMs", o.BackoffBaseMs)
	setIntQuery(q, "backoffMaxMs", o.BackoffMaxMs)
	setIntQuery(q, "backoffJitterMs", o.BackoffJitterMs)
	setIntQuery(q, "timeoutMs", o.TimeoutMs)
	setIntQuery(q, "warmupDelayMs", o.WarmupDelayMs)
	setIntQuery(q, "warmupJitterMs", o.WarmupJitterMs)
	setBoolQuery(q, "profileOnlyOnRateLimit", o.ProfileOnlyOnRateLimit)
	setBoolQuery(q, "headful", o.Headful)
}

func setBoolQuery(q neturl.Values, key string, value *bool) {
	if value != nil {
		q.Set(key, strconv.FormatBool(*value))
	}
}

func setIntQuery(q neturl.Values, key string, value *int) {
	if value != nil {
		q.Set(key, strconv.Itoa(*value))
	}
}

func (c *Client) InstagramStalk(ctx context.Context, username string, options InstagramStalkOptions) (*InstagramStalkResult, error) {
	u, err := neturl.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/api/instagram/stalk"

	q := u.Query()
	q.Set("username", username)
	options.apply(q)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, apiHTTPStatusError("instagram stalk", resp.StatusCode, body)
	}

	var out APIResponse[InstagramStalkResult]
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}

	if out.Data.Profile.Username == "" {
		return nil, fmt.Errorf("instagram stalk api tidak mengembalikan profile")
	}

	return &out.Data, nil
}
