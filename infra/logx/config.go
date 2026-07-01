package logx

type (
	// A LogConf is a logging config.
	LogConf struct {
		// ServiceName represents the service name.
		ServiceName string `config:",optional"`
		// Mode represents the logging mode, default is `console`.
		// console: log to console.
		// file: log to file.
		// volume: used in k8s, prepend the hostname to the log file name.
		Mode string `config:",default=console,options=[console,file,volume]"`
		// Encoding represents the encoding type, default is `json`.
		// json: json encoding.
		// plain: plain text encoding, typically used in development.
		Encoding string `config:",default=json,options=[json,plain]"`
		// TimeFormat represents the time format, default is `2006-01-02T15:04:05.000Z07:00`.
		TimeFormat string `config:",optional"`
		// Path represents the log file path, default is `logs`.
		Path string `config:",default=logs"`
		// Level represents the log level, default is `info`.
		Level string `config:",default=info,options=[debug,info,error,severe]"`
		// MaxContentLength represents the max content bytes, default is no limit.
		MaxContentLength uint32 `config:",optional"`
		// Compress represents whether to compress the log file, default is `false`.
		Compress bool `config:",optional"`
		// Stat represents whether to log statistics, default is `true`.
		Stat bool `config:",default=true"`
		// KeepDays represents how many days the log files will be kept. Default to keep all files.
		// Only take effect when Mode is `file` or `volume`, both work when Rotation is `daily` or `size`.
		KeepDays int `config:",optional"`
		// StackCooldownMillis represents the cooldown time for stack logging, default is 100ms.
		StackCooldownMillis int `config:",default=100"`
		// MaxBackups represents how many backup log files will be kept. 0 means all files will be kept forever.
		// Only take effect when RotationRuleType is `size`.
		// Even though `MaxBackups` sets 0, log files will still be removed
		// if the `KeepDays` limitation is reached.
		MaxBackups int `config:",default=0"`
		// MaxSize represents how much space the writing log file takes up. 0 means no limit. The unit is `MB`.
		// Only take effect when RotationRuleType is `size`
		MaxSize int `config:",default=0"`
		// Rotation represents the type of log rotation rule. Default is `daily`.
		// daily: daily rotation.
		// size: size limited rotation.
		Rotation string `config:",default=daily,options=[daily,size]"`
		// FileTimeFormat represents the time format for file name, default is `2006-01-02T15:04:05.000Z07:00`.
		FileTimeFormat string `config:",optional"`
		// FieldKeys represents the field keys.
		FieldKeys fieldKeyConf `config:",optional"`
	}

	fieldKeyConf struct {
		// CallerKey represents the caller key.
		CallerKey string `config:",default=caller"`
		// ContentKey represents the content key.
		ContentKey string `config:",default=content"`
		// DurationKey represents the duration key.
		DurationKey string `config:",default=duration"`
		// LevelKey represents the level key.
		LevelKey string `config:",default=level"`
		// SpanKey represents the span key.
		SpanKey string `config:",default=span"`
		// TimestampKey represents the timestamp key.
		TimestampKey string `config:",default=@timestamp"`
		// TraceKey represents the trace key.
		TraceKey string `config:",default=trace"`
		// TruncatedKey represents the truncated key.
		TruncatedKey string `config:",default=truncated"`
	}
)
