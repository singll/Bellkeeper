package defaults

const (
	// DefaultTagColor is the default color for newly created tags.
	DefaultTagColor = "#409EFF"

	// DefaultParserID is the default RagFlow document parser.
	DefaultParserID = "naive"

	// DefaultFetchInterval is the default RSS fetch interval in minutes.
	DefaultFetchInterval = 60

	// DefaultWebhookMethod is the default HTTP method for webhooks.
	DefaultWebhookMethod = "POST"

	// DefaultWebhookContentType is the default content type for webhooks.
	DefaultWebhookContentType = "application/json"

	// DefaultWebhookTimeout is the default webhook timeout in seconds.
	DefaultWebhookTimeout = 30

	// HealthCheckTimeout is the timeout for external service health checks in seconds.
	HealthCheckTimeout = 5
)
