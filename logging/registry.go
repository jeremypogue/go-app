package logging

import (
	"sort"
	"strings"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/spf13/viper"
)

// RootName of the root logger, defaults to root
var RootName string

func init() {
	RootName = "root"
}

// LoggerRegistry represents a registry for known loggers
type LoggerRegistry struct {
	config *viper.Viper
	store  map[string]Logger
	lock   *sync.Mutex
}

// NewRegistry creates a new logger registry
func NewRegistry(cfg *viper.Viper, context logrus.Fields) *LoggerRegistry {
	c := cfg
	if c.InConfig("logging") {
		c = cfg.Sub("logging")
	}

	keys := c.AllKeys()
	store := make(map[string]Logger, len(keys))
	reg := &LoggerRegistry{
		store:  store,
		config: c,
		lock:   new(sync.Mutex),
	}

	for _, k := range keys {
		// no sharing of context, so copy
		fields := make(logrus.Fields, len(context)+1)
		for kk, vv := range context {
			fields[kk] = vv
		}

		v := c
		if c.InConfig(k) {
			v = c.Sub(k)
		}

		addLoggingDefaults(v)
		fields["module"] = k
		if v.IsSet("name") {
			fields["module"] = v.GetString("name")
		}

		l := newNamedLogger(k, fields, v, nil)
		l.reg = reg
		reg.store[k] = l
	}
	if len(keys) == 0 {
		fields := make(logrus.Fields, len(context)+1)
		for k, v := range context {
			fields[k] = v
		}

		fields["module"] = RootName
		l := newNamedLogger(RootName, fields, c, nil)
		l.reg = reg
		reg.store[RootName] = l
	}

	return reg
}

// Get a logger by name, returns nil when logger doesn't exist.
// GetOK is the safe method to use.
func (r *LoggerRegistry) Get(name string) Logger {
	l, ok := r.GetOK(name)
	if !ok {
		return nil
	}
	return l
}

// GetOK a logger by name, boolean is true when a logger was found
func (r *LoggerRegistry) GetOK(name string) (Logger, bool) {
	r.lock.Lock()
	res, ok := r.store[strings.ToLower(name)]
	r.lock.Unlock()
	return res, ok
}

// Register a logger in this registry, overrides existing keys
func (r *LoggerRegistry) Register(path string, logger Logger) {
	r.lock.Lock()
	r.store[strings.ToLower(path)] = logger
	r.lock.Unlock()
}

// Root returns the root logger, the name is configurable through the RootName variable
func (r *LoggerRegistry) Root() Logger {
	return r.Get(RootName)
}

// Reload all the loggers with the new config
func (r *LoggerRegistry) Reload() {
	r.lock.Lock()
	defer r.lock.Unlock()

	// Get all keys, sorted by name and shortest to longest
	var keys []string
	for key := range r.store {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// find matching config
	// for each key find the longest possible path that has a config
	// if no more path found or parts are exhausted use last config and stop searching
	configs := make(map[string]*viper.Viper, len(keys))
	for _, key := range keys {
		configs[key] = findLongestMatchingPath(key, r.config)
	}

	// call reconfigure on logger
	for _, k := range keys {
		logger := r.store[k]
		if cfg, ok := configs[k]; ok {
			logger.Configure(cfg)
		}
	}
}

func findLongestMatchingPath(path string, cfg *viper.Viper) *viper.Viper {
	parts := strings.Split(path, ".")
	pl := len(parts)
	for i := range parts {
		mod := pl - i
		k := strings.Join(parts[:mod], ".")
		if cfg.IsSet(k) {
			return cfg.Sub(k)
		}
	}
	return nil
}
