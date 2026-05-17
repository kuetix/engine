package helpers

import (
	"encoding/json"
	"testing"

	"github.com/kuetix/helpers"
	"github.com/kuetix/logger"
)

func TestMergeMapsLevel0(t *testing.T) {
	options := []map[string]interface{}{
		{
			"debug": true,
		},
	}
	defaultOptions := map[string]interface{}{
		"debug":           false,
		"preventLogError": false,
		"preventLogInfo":  false,
	}

	options = append([]map[string]interface{}{defaultOptions}, options...)
	opts := helpers.MergeMapsLevel0(options...)
	output, err := json.Marshal(opts)
	if err != nil {
		logger.Debug(err)
	}
	logger.Debug("TestMergeMapsLevel0", string(output))
}
