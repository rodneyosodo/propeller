package sdk

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/absmach/propeller/pkg/proplet"
)

const propletsEndpoint = "/proplets"

func (sdk *propSDK) GetPropletAliveHistory(id string, offset, limit uint64) (proplet.PropletAliveHistoryPage, error) {
	url := fmt.Sprintf("%s%s/%s/alive-history?offset=%d&limit=%d", sdk.managerURL, propletsEndpoint, id, offset, limit)

	body, err := sdk.processRequest(http.MethodGet, url, nil, http.StatusOK)
	if err != nil {
		return proplet.PropletAliveHistoryPage{}, err
	}

	var page proplet.PropletAliveHistoryPage
	if err := json.Unmarshal(body, &page); err != nil {
		return proplet.PropletAliveHistoryPage{}, err
	}

	return page, nil
}

func (sdk *propSDK) DeleteProplet(id string) error {
	url := sdk.managerURL + propletsEndpoint + "/" + id

	if _, err := sdk.processRequest(http.MethodDelete, url, nil, http.StatusNoContent); err != nil {
		return err
	}

	return nil
}
