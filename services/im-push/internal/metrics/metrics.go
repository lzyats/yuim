package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	OnlineConns = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "im_push_online_conns",
		Help: "Current online websocket connections (approx).",
	})

	WSPushOK = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "im_push_ws_push_ok_total",
		Help: "Total ws messages queued successfully.",
	})
	WSPushBackpressure = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "im_push_ws_backpressure_total",
		Help: "Total times outbound queue was full (429/backpressure).",
	})
	WSPushOffline = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "im_push_ws_offline_total",
		Help: "Total times uid had no active connection (404/offline).",
	})

	BatchPushReq = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "im_push_batch_req_total",
		Help: "Total batch push requests received.",
	})
	BatchItems = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "im_push_batch_items_total",
		Help: "Total items received in batch push requests.",
	})
)

func Register() {
	prometheus.MustRegister(
		OnlineConns,
		WSPushOK, WSPushBackpressure, WSPushOffline,
		BatchPushReq, BatchItems,
	)
}
