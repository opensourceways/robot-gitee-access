package main

import (
	"flag"
	"net/http"
	"os"
	"strconv"

	"github.com/opensourceways/robot-gitee-plugin-lib/config"
	"github.com/opensourceways/robot-gitee-plugin-lib/interrupts"
	"github.com/opensourceways/robot-gitee-plugin-lib/logrusutil"
	liboptions "github.com/opensourceways/robot-gitee-plugin-lib/options"
	"github.com/opensourceways/robot-gitee-plugin-lib/secret"
	"github.com/sirupsen/logrus"
)

type options struct {
	plugin         liboptions.PluginOptions
	hmacSecretFile string
}

func (o *options) Validate() error {
	return o.plugin.Validate()
}

func gatherOptions(fs *flag.FlagSet, args ...string) options {
	var o options

	o.plugin.AddFlags(fs)

	fs.StringVar(&o.hmacSecretFile, "hmac-secret-file", "/etc/webhook/hmac", "Path to the file containing the HMAC secret.")

	fs.Parse(args)
	return o
}

const component = "robot-gitee-access"

func main() {
	logrusutil.ComponentInit(component)

	o := gatherOptions(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Args[1:]...)
	if err := o.Validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}

	configAgent := config.NewConfigAgent(func() config.PluginConfig {
		return new(configuration)
	})
	if err := configAgent.Start(o.plugin.PluginConfig); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	agent := demuxConfigAgent{agent: &configAgent}
	agent.Start()

	secretAgent := new(secret.Agent)
	if err := secretAgent.Start([]string{o.hmacSecretFile}); err != nil {
		logrus.WithError(err).Fatal("Error starting secret agent.")
	}

	gethmac := secretAgent.GetTokenGenerator(o.hmacSecretFile)

	d := dispatcher{
		agent: &agent,
		hmac: func() string {
			return string(gethmac())
		},
	}

	defer interrupts.WaitForGracefulShutdown()

	interrupts.OnInterrupt(func() {
		agent.Stop()
		configAgent.Stop()
		secretAgent.Stop()
		d.Wait()
	})

	// Return 200 on / for health checks.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {})

	// For /hook, handle a webhook normally.
	http.Handle("/gitee-hook", &d)

	httpServer := &http.Server{Addr: ":" + strconv.Itoa(o.plugin.Port)}

	interrupts.ListenAndServe(httpServer, o.plugin.GracePeriod)
}
