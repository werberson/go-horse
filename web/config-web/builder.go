package web

import (
	"fmt"
	"github.com/labbsr0x/go-horse/filters"
	"github.com/docker/docker/api"
	"github.com/sirupsen/logrus"
	"net/http"
	"time"

	sockclient "github.com/labbsr0x/go-horse/sockClient"

	"github.com/docker/docker/client"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// All envs that GHP need to work with
const (
	dockerAPIVersion = "docker-api-version"
	dockerSockURL    = "docker-sock-url"
	targetHostName   = "target-host-name"
	logLevel         = "log-level"
	port             = "port"
	shutdownTime   = "shutdown-time"
)

// Flags define the fields that will be passed via cmd
type Flags struct {
	DockerAPIVersion string
	DockerSockURL    string
	TargetHostName   string
	LogLevel         string
	Port             string
	ShutdownTime   time.Duration
}

// WebBuilder defines the parametric information of a gohorse server instance
type WebBuilder struct {
	*Flags
	DockerCli  *client.Client
	SockClient *http.Client
	Filter     *filters.FilterManager
}

// AddFlags adds flags for Builder.
func AddFlags(flags *pflag.FlagSet) {
	flags.StringP(dockerAPIVersion, "v", api.DefaultVersion, "[optional] Version of Docker API. Defaults to " + api.DefaultVersion)
	flags.StringP(dockerSockURL, "u", client.DefaultDockerHost, "[optional] URL of Docker Socket. Defaults to " + client.DefaultDockerHost)
	flags.StringP(targetHostName, "n", "", "Target host name")
	flags.StringP(logLevel, "l", "info", "[optional] Sets the Log Level to one of seven (trace, debug, info, warn, error, fatal, panic). Defaults to info")
	flags.StringP(port, "p", ":8080", "[optional] Go Horse port. Defaults to :8080")
	flags.StringP(shutdownTime, "t", "5", "[optional] Sets the Graceful Shutdown wait time (seconds). Defaults to 5")
}

// InitFromWebBuilder initializes the web server builder with properties retrieved from Viper.
func (b *WebBuilder) InitFromViper(v *viper.Viper, filter *filters.FilterManager) *WebBuilder {

	flags := new(Flags)
	flags.DockerAPIVersion = v.GetString(dockerAPIVersion)
	flags.DockerSockURL = v.GetString(dockerSockURL)
	flags.TargetHostName = v.GetString(targetHostName)
	flags.LogLevel = v.GetString(logLevel)
	flags.Port = v.GetString(port)
	flags.ShutdownTime = v.GetDuration(shutdownTime)

	flags.check()
	flags.setLog()

	b.Flags = flags
	b.DockerCli = b.getDockerCli()
	b.SockClient = b.getSocketClient()
	b.Filter = filter

	return b
}

func (flags *Flags) check() {

	logrus.Infof("Web Flags: %v", flags)

	haveEmptyRequiredFlags := flags.DockerSockURL == "" ||
		flags.TargetHostName == ""

	requiredFlagsNames := []string{
		dockerSockURL,
		targetHostName,
	}

	if haveEmptyRequiredFlags {
		msg := fmt.Sprintf("The following flags cannot be empty:")
		for _, name := range requiredFlagsNames {
			msg += fmt.Sprintf("\n\t%v", name)
		}
		panic(msg)
	}

}

func (b *WebBuilder) getDockerCli() *client.Client {

	dockerCli, err := client.NewClientWithOpts(client.WithVersion(b.Flags.DockerAPIVersion), client.WithHost(b.Flags.DockerSockURL))

	if err != nil {
		panic(err)
	}

	return dockerCli
}

func (b *WebBuilder) getSocketClient() *http.Client {
	return sockclient.Get(b.Flags.DockerSockURL)
}

func (f *Flags) setLog() {

	level, err := logrus.ParseLevel(f.LogLevel)

	if err != nil {
		panic(err)
	}
	logrus.WithFields(logrus.Fields{
		"Log Level": f.LogLevel,
	}).Infof("Setting log level")

	logrus.SetLevel(level)
}
