package rules

// ConditionNode is a node in the condition DSL tree.
// Branch: Operator + Conditions (AND/OR) or Operator + Condition (NOT).
// Leaf:   Field + Op + Value.
type ConditionNode struct {
	// Branch
	Operator   string          `json:"operator"`
	Conditions []ConditionNode `json:"conditions"`
	Condition  *ConditionNode  `json:"condition"` // for NOT

	// Leaf
	Field string      `json:"field"`
	Op    string      `json:"op"`
	Value interface{} `json:"value"`
}

// Action is a single action executed when a rule fires.
type Action struct {
	Type string `json:"type"` // downlink | notify | webhook | set_shadow

	// downlink
	ActionName   string                 `json:"action"`
	Payload      map[string]interface{} `json:"payload"`
	TargetSerial string                 `json:"target_serial"`

	// notify
	ChannelID int64  `json:"channel_id"`
	Title     string `json:"title"`
	Body      string `json:"body"`

	// webhook
	URL          string            `json:"url"`
	Method       string            `json:"method"`
	Headers      map[string]string `json:"headers"`
	BodyTemplate string            `json:"body_template"`

	// set_shadow
	Desired map[string]interface{} `json:"desired"`
}

// Rule mirrors the Django Rule model, cached in Redis.
type Rule struct {
	ID                   int64         `json:"id"`
	TenantID             int64         `json:"tenant"`
	Name                 string        `json:"name"`
	TriggerStreamPattern string        `json:"trigger_stream_pattern"`
	Conditions           ConditionNode `json:"conditions"`
	Actions              []Action      `json:"actions"`
	CooldownSeconds      int           `json:"cooldown_seconds"`
	Enabled              bool          `json:"enabled"`
}

// MessageContext carries parsed info about an incoming MQTT message.
type MessageContext struct {
	TenantID int64
	Serial   string
	Stream   string
	Payload  map[string]interface{}
	RawTopic string
}

// ExecStatus mirrors RuleExecution.Status in Django.
type ExecStatus string

const (
	StatusTriggered ExecStatus = "triggered"
	StatusCooldown  ExecStatus = "cooldown_skipped"
	StatusError     ExecStatus = "error"
)
