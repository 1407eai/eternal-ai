package main_test

import (
	"crypto/tls"
	"net/http"
	"testing"

	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/configs"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/daos"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/databases"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/services"
)

var ts *services.Service

func init() {
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	conf := configs.GetConfig()
	dbMain, err := databases.Init(
		conf.DbURL,
		nil,
		1,
		20,
		conf.Debug,
	)
	if err != nil {
		panic(err)
	}
	daos.InitDBConn(
		dbMain,
	)
	var (
		s = services.NewService(
			conf,
		)
	)
	ts = s
}

func Test_SRV(t *testing.T) {
}
