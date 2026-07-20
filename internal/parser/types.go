package parser

import "nexusbridge/internal/requestpolicy"

type SiteParseOptions struct {
	SiteID        string
	BaseURL       string
	URL           string
	Selectors     ParseSelectors
	TorrentFields map[string]SiteFieldDefinition
}

type ParsedPage struct {
	SiteConfig   SiteConfig     `json:"site_config"`
	Selectors    ParseSelectors `json:"selectors"`
	SearchConfig SearchConfig   `json:"search_config"`
	Torrents     []TorrentEntry `json:"torrents"`
	Pagination   Pagination     `json:"pagination"`
}

type TorrentDetailParseOptions struct {
	SiteID    string
	TorrentID string
	BaseURL   string
	URL       string
}

type TorrentDetail struct {
	SiteID            string `json:"site_id"`
	TorrentID         string `json:"torrent_id"`
	DetailURL         string `json:"detail_url"`
	DetailTitle       string `json:"detail_title"`
	Subtitle          string `json:"subtitle"`
	ProductURL        string `json:"product_url"`
	InfoHash          string `json:"info_hash"`
	DetailDescription string `json:"detail_description"`
	DetailRawText     string `json:"detail_raw_text"`
}

type SiteDefinition struct {
	ID            string               `json:"id"`
	Name          string               `json:"name"`
	Domain        string               `json:"domain"`
	Encoding      string               `json:"encoding,omitempty"`
	Public        bool                 `json:"public"`
	HTML          SiteHTMLDefinition   `json:"html"`
	DetailPageURL string               `json:"detail_page_url,omitempty"`
	Favicon       string               `json:"favicon,omitempty"`
	RequestRules  []requestpolicy.Rule `json:"request_rules,omitempty"`
}

type SiteHTMLDefinition struct {
	Search   SiteSearchDefinition              `json:"search"`
	Category map[string][]SiteCategory         `json:"category,omitempty"`
	Torrents SiteTorrentsDefinition            `json:"torrents"`
	Browse   SiteBrowseDefinition              `json:"browse,omitempty"`
	Conf     map[string][]string               `json:"conf,omitempty"`
	Extra    map[string]map[string]interface{} `json:"-"`
}

type SiteSearchDefinition struct {
	Paths  []SiteSearchPath       `json:"paths"`
	Params map[string]interface{} `json:"params,omitempty"`
	Batch  map[string]interface{} `json:"batch,omitempty"`
	Fields SiteSearchFields       `json:"fields,omitempty"`
}

type SiteSearchPath struct {
	Path   string `json:"path"`
	Method string `json:"method"`
}

type SiteCategory struct {
	ID          int    `json:"id"`
	Cat         string `json:"cat"`
	Desc        string `json:"desc"`
	Default     bool   `json:"default,omitempty"`
	SearchField string `json:"search_field,omitempty"`
}

type SiteTorrentsDefinition struct {
	List         SiteListDefinition             `json:"list"`
	Fields       map[string]SiteFieldDefinition `json:"fields,omitempty"`
	BrowseList   SiteListDefinition             `json:"browse_list,omitempty"`
	BrowseFields map[string]SiteFieldDefinition `json:"browse_fields,omitempty"`
}

type SiteListDefinition struct {
	Selector string `json:"selector"`
}

type SiteFieldDefinition map[string]interface{}

type SiteBrowseDefinition struct {
	Path  string `json:"path,omitempty"`
	Start int    `json:"start,omitempty"`
}

type SiteConfig struct {
	SiteID        string               `json:"site_id"`
	Name          string               `json:"name,omitempty"`
	BaseURL       string               `json:"base_url"`
	Domain        string               `json:"domain,omitempty"`
	DomainAliases []string             `json:"domain_aliases,omitempty"`
	Encoding      string               `json:"encoding,omitempty"`
	URL           string               `json:"url"`
	SearchPath    string               `json:"search_path"`
	DownloadPath  string               `json:"download_path"`
	Pagination    SitePaginationConfig `json:"pagination,omitempty"`
	QueryTemplate map[string]string    `json:"query_template"`
}

// SitePaginationConfig 描述由站点定义派生的数字分页规则。
type SitePaginationConfig struct {
	Parameter string `json:"parameter,omitempty"`
	Query     string `json:"query,omitempty"`
	Start     int    `json:"start"`
}

type ParseSelectors struct {
	SearchBox   string `json:"searchbox"`
	TorrentRows string `json:"torrent_rows"`
	TorrentName string `json:"torrent_name"`
	Pagination  string `json:"pagination"`
}

type SearchConfig struct {
	Categories []SearchOption   `json:"categories"`
	Tags       []SearchOption   `json:"tags"`
	Checkboxes []SearchGroup    `json:"checkboxes"`
	Selects    []SelectField    `json:"selects"`
	Ranges     []RangeField     `json:"ranges"`
	Keyword    KeywordField     `json:"keyword"`
	Fields     SiteSearchFields `json:"fields,omitempty"`
}

type SiteSearchFields struct {
	Checkboxes []SiteSearchCheckboxGroup `json:"checkboxes,omitempty"`
	Tags       *SiteSearchField          `json:"tags,omitempty"`
	Selects    []SiteSearchField         `json:"selects,omitempty"`
	Ranges     []SiteSearchField         `json:"ranges,omitempty"`
	Keyword    *SiteSearchField          `json:"keyword,omitempty"`
	Page       *SiteSearchField          `json:"page,omitempty"`
}

func (fields SiteSearchFields) Empty() bool {
	return len(fields.Checkboxes) == 0 && fields.Tags == nil && len(fields.Selects) == 0 && len(fields.Ranges) == 0 && fields.Keyword == nil && fields.Page == nil
}

type SiteSearchCheckboxGroup struct {
	Name    string                  `json:"name"`
	Type    string                  `json:"type"`
	Label   string                  `json:"label"`
	Options []SiteSearchFieldOption `json:"options"`
}

type SiteSearchField struct {
	Name      string                  `json:"name"`
	Type      string                  `json:"type"`
	Label     string                  `json:"label"`
	Role      string                  `json:"role,omitempty"`
	Query     string                  `json:"query,omitempty"`
	Begin     string                  `json:"begin,omitempty"`
	End       string                  `json:"end,omitempty"`
	Exclusive bool                    `json:"exclusive,omitempty"`
	Options   []SiteSearchFieldOption `json:"options,omitempty"`
}

type SiteSearchFieldOption struct {
	Name        string `json:"name,omitempty"`
	Value       string `json:"value"`
	Label       string `json:"label"`
	Query       string `json:"query,omitempty"`
	FilterValue string `json:"filter_value,omitempty"`
}

type SearchGroup struct {
	Name    string         `json:"name"`
	Label   string         `json:"label"`
	Options []SearchOption `json:"options"`
}

type SearchOption struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Label     string `json:"label"`
	QueryName string `json:"query_name"`
	Query     string `json:"query"`
}

type SelectField struct {
	Name    string         `json:"name"`
	Label   string         `json:"label"`
	Options []SearchOption `json:"options"`
}

type RangeField struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Begin string `json:"begin"`
	End   string `json:"end"`
	Kind  string `json:"kind"`
}

type KeywordField struct {
	Name              string         `json:"name"`
	AreaSelectName    string         `json:"area_select_name"`
	AreaOptions       []SearchOption `json:"area_options"`
	ModeSelectName    string         `json:"mode_select_name"`
	ModeOptions       []SearchOption `json:"mode_options"`
	SuggestionQueries []SearchOption `json:"suggestion_queries"`
}

type Pagination struct {
	CurrentLabel string     `json:"current_label"`
	Pages        []PageLink `json:"pages"`
}

type PageLink struct {
	Page  string `json:"page"`
	Label string `json:"label"`
	Href  string `json:"href"`
	URL   string `json:"url"`
}

type TorrentEntry struct {
	SiteID             string   `json:"site_id"`
	ID                 int      `json:"id"`
	Category           string   `json:"category"`
	CategoryQuery      string   `json:"category_query"`
	Title              string   `json:"title"`
	DetailHref         string   `json:"detail_href"`
	DetailURL          string   `json:"detail_url"`
	DownloadHref       string   `json:"download_href"`
	DownloadURL        string   `json:"download_url"`
	CoverURL           string   `json:"cover_url"`
	Tags               []string `json:"tags"`
	TagIDs             []string `json:"tag_ids"`
	Promotion          string   `json:"promotion"`
	PromotionClass     string   `json:"promotion_class"`
	PromotionEndsAt    string   `json:"promotion_ends_at"`
	PromotionRemaining string   `json:"promotion_remaining"`
	Subtitle           string   `json:"subtitle"`
	Description        string   `json:"description"`
	SizeText           string   `json:"size_text"`
	SizeBytes          int64    `json:"size_bytes"`
	Seeders            int      `json:"seeders"`
	Leechers           int      `json:"leechers"`
	Snatches           int      `json:"snatches"`
	Comments           int      `json:"comments"`
	PublishedAt        string   `json:"published_at"`
	PublishedText      string   `json:"published_text"`
	StickyLevel        int      `json:"sticky_level"`
	Bookmarked         bool     `json:"bookmarked"`
}
