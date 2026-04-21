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

	// DefaultMatrixDomain is the default Matrix server domain.
	DefaultMatrixDomain = "matrix.singll.net"

	// DefaultPageSize is the default pagination size for frontend lists.
	DefaultPageSize = 20
)

// Parse queue defaults (ragflow_parse_queue.go).
const (
	// ParseQueueDefaultBatchSize is the initial batch size for smart parsing.
	ParseQueueDefaultBatchSize = 3

	// ParseQueueDefaultInitialDelay is the initial delay between parsing batches in seconds.
	ParseQueueDefaultInitialDelay = 15

	// ParseQueueDefaultPollInterval is the interval between status polls in seconds.
	ParseQueueDefaultPollInterval = 10

	// ParseQueueDefaultSoftTimeout is the max seconds for normal polling window.
	ParseQueueDefaultSoftTimeout = 300

	// ParseQueueDefaultMaxRecoveryAttempts is the max recovery attempts per document.
	ParseQueueDefaultMaxRecoveryAttempts = 3
)

// Parser profile defaults (ragflow_http.go).
const (
	// ParserDefaultChunkTokenNum is the default chunk token count.
	ParserDefaultChunkTokenNum = 64

	// ParserDefaultDelimiter is the default sentence delimiter for chunking.
	ParserDefaultDelimiter = "\n!?;。？！"

	// ParserDefaultTopNTags is the default number of top tags to extract.
	ParserDefaultTopNTags = 3
)
