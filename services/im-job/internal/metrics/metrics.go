package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	Duplicates = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "im_job_duplicates_total",
		Help: "Total duplicate events dropped by msg_id dedupe.",
	})

	Consumed = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "im_job_mq_consumed_total",
		Help: "Total MQ messages consumed (events).",
	})
	EventDecodeFail = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "im_job_event_decode_fail_total",
		Help: "Total event decode failures.",
	})
	DeliverOK = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "im_job_deliver_ok_total",
		Help: "Total successful online deliveries.",
	})
	DeliverFail = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "im_job_deliver_fail_total",
		Help: "Total delivery failures (includes store/route/send errors).",
	})
	OfflineEnqueued = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "im_job_offline_enqueued_total",
		Help: "Total offline enqueued packets.",
	})
	BatchSent = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "im_job_comet_batch_sent_total",
		Help: "Total comet batch requests sent.",
	})
	BatchItems = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "im_job_comet_batch_items_total",
		Help: "Total items delivered via batch requests.",
	})
	BreakerOpen = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "im_job_breaker_open_total",
		Help: "Total times a circuit breaker opened for a comet node.",
	})
	BreakerDrop = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "im_job_breaker_drop_total",
		Help: "Total items skipped due to breaker open (sent to offline directly).",
	})
)

func Register() {
	prometheus.MustRegister(
		Duplicates,
		Consumed, EventDecodeFail,
		DeliverOK, DeliverFail, OfflineEnqueued,
		BatchSent, BatchItems,
		BreakerOpen, BreakerDrop,
	)
}
