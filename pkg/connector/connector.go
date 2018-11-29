package connector

import (
	"github.com/robaho/go-trader/pkg/common"
	"github.com/robaho/go-trader/pkg/connector/grpc"
	"github.com/robaho/go-trader/pkg/connector/marketdata"
	"github.com/robaho/go-trader/pkg/connector/qfix"
	"io"
)

func NewConnector(callback common.ConnectorCallback, props common.Properties, logOutput io.Writer) common.ExchangeConnector {
	var c common.ExchangeConnector

	if "grpc" == props.GetString("protocol", "fix") {
		c = grpc.NewConnector(callback, props, logOutput)
	} else {
		c = qfix.NewConnector(callback, props, logOutput)
	}

	marketdata.StartMarketDataReceiver(c, callback, props, logOutput)
	return c
}
