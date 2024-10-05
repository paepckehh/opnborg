package opnborg

type firmwareStatus struct {
	Product struct {
		ProductAbi            string `json:"product_abi"`
		ProductArch           string `json:"product_arch"`
		ProductCheck          any    `json:"product_check"`
		ProductConflicts      string `json:"product_conflicts"`
		ProductCopyrightOwner string `json:"product_copyright_owner"`
		ProductCopyrightURL   string `json:"product_copyright_url"`
		ProductCopyrightYears string `json:"product_copyright_years"`
		ProductEmail          string `json:"product_email"`
		ProductHash           string `json:"product_hash"`
		ProductID             string `json:"product_id"`
		ProductLatest         string `json:"product_latest"`
		ProductLicense        []any  `json:"product_license"`
		ProductLog            int    `json:"product_log"`
		ProductMirror         string `json:"product_mirror"`
		ProductName           string `json:"product_name"`
		ProductNickname       string `json:"product_nickname"`
		ProductRepos          string `json:"product_repos"`
		ProductSeries         string `json:"product_series"`
		ProductTier           string `json:"product_tier"`
		ProductTime           string `json:"product_time"`
		ProductVersion        string `json:"product_version"`
		ProductWebsite        string `json:"product_website"`
	} `json:"product"`
	StatusMsg string `json:"status_msg"`
	Status    string `json:"status"`
}
