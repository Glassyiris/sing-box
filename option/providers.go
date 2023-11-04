package option

type ProvidersOptions struct {
	Type          int    `json:"type"`
	Detour        string `json:"detour,omitempty"`
	Url           string `json:"url"`
	Path          string `json:"path"`
	Interval      int64  `json:"interval,omitempty"`
	Filter        string `json:"filter,omitempty"`
	ExcludeFilter string `json:"exclude-filter,omitempty"`
}
