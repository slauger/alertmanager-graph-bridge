package server

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds the Prometheus collectors exposed at /metrics.
type Metrics struct {
	WebhookRequests *prometheus.CounterVec
	WebhookDuration prometheus.Histogram
	MailsSent       prometheus.Counter
	MailSendErrors  *prometheus.CounterVec
	SendDuration    prometheus.Histogram
	PanicsRecovered prometheus.Counter
}

// NewMetrics registers the application metrics, plus the standard Go and
// process collectors, on the given registry.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	_ = reg.Register(collectors.NewGoCollector())
	_ = reg.Register(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	factory := promauto.With(reg)
	return &Metrics{
		WebhookRequests: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "agb_webhook_requests_total",
			Help: "Total number of Alertmanager webhook requests by outcome.",
		}, []string{"outcome"}),
		WebhookDuration: factory.NewHistogram(prometheus.HistogramOpts{
			Name:    "agb_webhook_request_duration_seconds",
			Help:    "Latency of Alertmanager webhook request handling in seconds.",
			Buckets: prometheus.DefBuckets,
		}),
		MailsSent: factory.NewCounter(prometheus.CounterOpts{
			Name: "agb_mails_sent_total",
			Help: "Total number of e-mails sent successfully via Microsoft Graph.",
		}),
		MailSendErrors: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "agb_mail_send_errors_total",
			Help: "Total number of failed e-mail send attempts by reason.",
		}, []string{"reason"}),
		SendDuration: factory.NewHistogram(prometheus.HistogramOpts{
			Name:    "agb_mail_send_duration_seconds",
			Help:    "Latency of Microsoft Graph sendMail calls in seconds.",
			Buckets: prometheus.DefBuckets,
		}),
		PanicsRecovered: factory.NewCounter(prometheus.CounterOpts{
			Name: "agb_panics_recovered_total",
			Help: "Total number of panics recovered by the HTTP middleware.",
		}),
	}
}
