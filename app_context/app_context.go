package app_context

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/comstud/go-rollbar/rollbar"
	_ "github.com/lib/pq"
	"github.com/tilteng/go-logger/logger"
	"github.com/tilteng/go-metrics/metrics"
)

type AppContext interface {
	APIPort() int
	AppName() string
	BaseExternalURL() string
	CodeVersion() string
	DB() *sql.DB
	Hostname() string
	JSONSchemaPath() string
	Logger() logger.CtxLogger
	MetricsClient() metrics.MetricsClient
	MetricsEnabled() bool
	RollbarClient() rollbar.Client
	RollbarEnabled() bool
	SetLogger(logger.CtxLogger) AppContext
}

type baseAppContext struct {
	apiPort         int
	appName         string
	baseExternalURL string
	codeVersion     string
	db              *sql.DB
	hostname        string
	jsonSchemaPath  string
	logger          logger.CtxLogger
	metricsClient   metrics.MetricsClient
	metricsEnabled  bool
	rollbarClient   rollbar.Client
	rollbarEnabled  bool
}

func (self *baseAppContext) APIPort() int {
	return self.apiPort
}

func (self *baseAppContext) AppName() string {
	return self.appName
}

func (self *baseAppContext) BaseExternalURL() string {
	return self.baseExternalURL
}

func (self *baseAppContext) CodeVersion() string {
	return self.codeVersion
}

func (self *baseAppContext) DB() *sql.DB {
	return self.db
}

func (self *baseAppContext) Hostname() string {
	return self.hostname
}

func (self *baseAppContext) JSONSchemaPath() string {
	return self.jsonSchemaPath
}

func (self *baseAppContext) Logger() logger.CtxLogger {
	return self.logger
}

func (self *baseAppContext) MetricsClient() metrics.MetricsClient {
	return self.metricsClient
}

func (self *baseAppContext) MetricsEnabled() bool {
	return self.metricsEnabled
}

func (self *baseAppContext) RollbarClient() rollbar.Client {
	return self.rollbarClient
}

func (self *baseAppContext) RollbarEnabled() bool {
	return self.rollbarEnabled
}

func (self *baseAppContext) SetLogger(logger logger.CtxLogger) AppContext {
	self.logger = logger
	return self
}

func (self *baseAppContext) isDisabled(s string) (bool, error) {
	s += "_DISABLE"
	if disable, ok := os.LookupEnv(s); ok {
		if disable == "true" {
			return true, nil
		}
		if disable != "false" {
			return true, errors.New(s + " must be 'true' or 'false'")
		}
	}
	return false, nil
}

func (self *baseAppContext) setRollbarClientFromEnv() error {
	if disabled, err := self.isDisabled("ROLLBAR"); disabled {
		return err
	}

	api_key := os.Getenv("ROLLBAR_API_KEY")
	if api_key == "" {
		return nil
	}

	if rcli, err := rollbar.NewClient(api_key); err != nil {
		return fmt.Errorf("Error creating rollbar client: %s", err)
	} else {
		self.rollbarClient = rcli
		self.rollbarEnabled = true

		opts := self.rollbarClient.Options()

		env := os.Getenv("ROLLBAR_ENVIRONMENT")
		if env == "development" || env == "staging" || env == "production" {
			opts.Environment = env
		}

		if len(self.codeVersion) != 0 {
			opts.NotifierServer.CodeVersion = self.codeVersion
		}
	}

	return nil
}

func (self *baseAppContext) setMetricsClientFromEnv() error {
	if disabled, err := self.isDisabled("METRICS"); disabled {
		return err
	}

	metrics_addr := os.Getenv("METRICS_ADDR")
	metrics_tags := os.Getenv("METRICS_TAGS")

	metrics_namespace, ok := os.LookupEnv("METRICS_NAMESPACE")
	if !ok {
		metrics_namespace = self.appName + "."
	}

	metrics_hostname, ok := os.LookupEnv("METRICS_HOSTNAME")
	if !ok {
		metrics_hostname = self.hostname
	}

	tags_map := map[string]string{
		"application": self.appName,
	}

	if len(metrics_hostname) > 0 {
		tags_map["host"] = metrics_hostname
	}

	for _, kv := range strings.Split(metrics_tags, ",") {
		if len(kv) == 0 {
			continue
		}
		if !strings.Contains(kv, "=") {
			kv += "="
		}
		parts := strings.SplitN(kv, "=", 2)
		if len(parts[0]) > 0 {
			tags_map[parts[0]] = parts[1]
		}
	}

	if mcli, err := metrics.NewMetricsClient(metrics_addr); err != nil {
		return err
	} else {
		mcli.SetNamespace(metrics_namespace)
		mcli.SetTags(tags_map)

		if err := mcli.Init(); err != nil {
			return err
		}

		self.metricsClient = mcli
		self.metricsEnabled = true
	}

	return nil
}

func (self *baseAppContext) setDBFromEnv() error {
	db_string := os.Getenv("DB_DSN")
	if len(db_string) == 0 {
		return nil
	}

	db, err := sql.Open("postgres", db_string)
	if err != nil {
		return errors.New("Couldn't open the database. Check that DB_DSN is correct.")
	}

	self.db = db

	return nil
}

func getIntFromEnv(name string) (int, bool, error) {
	str, found := os.LookupEnv(name)
	if !found || str == "" {
		return 0, found, nil
	}

	num, err := strconv.Atoi(str)
	if err != nil {
		return 0, true, fmt.Errorf("Env '%s' is not a number: %s", name, err)
	}

	return num, true, nil
}

func NewAppContext(app_name string) (AppContext, error) {
	appctx := &baseAppContext{
		logger:         logger.DefaultStdoutCtxLogger(),
		appName:        app_name,
		rollbarEnabled: false,
		rollbarClient:  rollbar.NewNOOPClient(),
		metricsEnabled: false,
		metricsClient:  metrics.NewNOOPClient(),
	}

	if host, err := os.Hostname(); err != nil {
		return nil, fmt.Errorf("Couldn't figure out hostname: %s", err)
	} else {
		appctx.hostname = host
	}

	// Set this before we setup rollbarClient
	appctx.codeVersion = os.Getenv("CODE_VERSION")

	if port, _, err := getIntFromEnv("API_PORT"); err != nil {
		return nil, err
	} else if port < 0 {
		return nil, errors.New("API_PORT can't be a negative number")
	} else {
		appctx.apiPort = port
	}

	appctx.jsonSchemaPath = os.Getenv("JSON_SCHEMA_PATH")
	appctx.baseExternalURL = os.Getenv("BASE_URL")

	if err := appctx.setMetricsClientFromEnv(); err != nil {
		return nil, fmt.Errorf("Error setting metrics client: %s", err)
	}

	if err := appctx.setRollbarClientFromEnv(); err != nil {
		return nil, fmt.Errorf("Error setting rollbar client: %s", err)
	}

	if err := appctx.setDBFromEnv(); err != nil {
		return nil, fmt.Errorf("Error setting DB object: %s", err)
	}

	return appctx, nil
}
