package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/opensourceways/community-robot-lib/config"
	"github.com/opensourceways/community-robot-lib/utils"
	"github.com/sirupsen/logrus"
)

type demuxConfigAgent struct {
	agent *config.ConfigAgent

	mut     sync.RWMutex
	demux   map[string]eventsDemux
	version string
	t       utils.Timer
}

func (ca *demuxConfigAgent) load() {
	v, c := ca.agent.GetConfig()
	if ca.version == v {
		return
	}

	nc, ok := c.(*configuration)
	if !ok {
		logrus.Errorf("can't convert to configuration")
		return
	}

	if nc == nil {
		logrus.Error("empty pointer of configuration")
		return
	}

	m := nc.Config.getDemux()

	// this function runs in serially, and ca.version is accessed in it,
	// so, there is no need to update it under the lock.
	ca.version = v

	ca.mut.Lock()
	ca.demux = m
	ca.mut.Unlock()
}

func (ca *demuxConfigAgent) getEndpoints(org, repo, event string) []string {
	ca.mut.RLock()
	v := getEventsDemux(org, repo, ca.demux)[event]
	ca.mut.RUnlock()

	return v
}

func getEventsDemux(org, repo string, demux map[string]eventsDemux) eventsDemux {
	if demux == nil {
		return eventsDemux{}
	}

	fullname := fmt.Sprintf("%s/%s", org, repo)
	if items, ok := demux[fullname]; ok {
		return items
	}

	if items, ok := demux[org]; ok {
		return items
	}

	return eventsDemux{}
}

func (ca *demuxConfigAgent) start() {
	ca.load()

	ca.t.Start(
		func() {
			ca.load()
		},
		1*time.Minute,
		0,
	)
}

func (ca *demuxConfigAgent) stop() {
	ca.t.Stop()
}
