package api

import "encoding/json"

// FetchGroups 获取所有分组
func (c *Client) FetchGroups() ([]Group, error) {
	apiResp, err := c.Get("/api/v1/admin/groups/all")
	if err != nil {
		return nil, err
	}

	var groups []Group
	if err := json.Unmarshal(apiResp.Data, &groups); err != nil {
		return nil, err
	}
	return groups, nil
}
