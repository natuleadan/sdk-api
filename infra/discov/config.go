package discov

import "errors"

var (
	// errEmptyEtcdHosts indicates that etcd hosts are empty.
	errEmptyEtcdHosts = errors.New("empty etcd hosts")
	// errEmptyEtcdKey indicates that etcd key is empty.
	errEmptyEtcdKey = errors.New("empty etcd key")
)

// EtcdConf is the config item with the given key on etcd.
type EtcdConf struct {
	Hosts              []string
	Key                string
	ID                 int64  `config:",optional"`
	User               string `config:",optional"`
	Pass               string `config:",optional"`
	CertFile           string `config:",optional"`
	CertKeyFile        string `config:",optional=CertFile"`
	CACertFile         string `config:",optional=CertFile"`
	InsecureSkipVerify bool   `config:",optional"`
}

// HasAccount returns if account provided.
func (c EtcdConf) HasAccount() bool {
	return len(c.User) > 0 && len(c.Pass) > 0
}

// HasID returns if ID provided.
func (c EtcdConf) HasID() bool {
	return c.ID > 0
}

// HasTLS returns if TLS CertFile/CertKeyFile/CACertFile are provided.
func (c EtcdConf) HasTLS() bool {
	return len(c.CertFile) > 0 && len(c.CertKeyFile) > 0 && len(c.CACertFile) > 0
}

// Validate validates c.
func (c EtcdConf) Validate() error {
	switch {
	case len(c.Hosts) == 0:
		return errEmptyEtcdHosts
	case len(c.Key) == 0:
		return errEmptyEtcdKey
	default:
		return nil
	}
}
