package memory

import (
	v1 "k8s.io/api/core/v1"
	"os"
	"path/filepath"
	"sync"

	"github.com/containerd/cgroups"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

type cgroupCh = chan *MemoryProtect

type cgroupNotifierCache struct {
	mu         sync.Mutex
	podNameMap map[string]struct{}
}

var notifierCache = cgroupNotifierCache{
	podNameMap: make(map[string]struct{}),
}

func (s *cgroupNotifierCache) add(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, e := s.podNameMap[key]; e {
		return false
	}
	s.podNameMap[key] = struct{}{}
	return true
}

func (s *cgroupNotifierCache) del(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.podNameMap, key)
}

type MemoryProtect struct {
	Pod                 *v1.Pod
	cgroup              *cgroups.Cgroup
	parentCgroup        *cgroups.Cgroup
	ThresholdRatio      float64
	ScalaRatio          float64
	MemoryThreshold     int64
	OldMemLimit         int64
	NewMemLimit         int64
	thresholdHitEventCh chan<- *MemoryProtect
	limitHitEventCh     chan<- *MemoryProtect
}

func updateMemLimit(cgroup *cgroups.Cgroup, newMemLimit int64) error {
	if err := (*cgroup).Update(&specs.LinuxResources{
		Memory: &specs.LinuxMemory{
			Swap: &newMemLimit,
		},
	}); err != nil {
		return err
	}
	if err := (*cgroup).Update(&specs.LinuxResources{
		Memory: &specs.LinuxMemory{
			Limit: &newMemLimit,
		},
	}); err != nil {
		return err
	}
	return nil
}

func parentPath(path cgroups.Path) cgroups.Path {
	return func(name cgroups.Name) (string, error) {
		p, err := path(name)
		if err != nil {
			return "", err
		}
		return filepath.Dir(p), nil
	}
}

func GetMemoryLimit(pid int) (uint64, error) {
	path := cgroups.PidPath(pid)
	control, err := cgroups.Load(cgroups.V1, path)
	if err != nil {
		logrus.Errorf("pid: %v cgroup load fail, %v", pid, err)
		return 0, err
	}
	stat, err := control.Stat(cgroups.IgnoreNotExist)
	if err != nil {
		return 0, err
	}
	return stat.Memory.HierarchicalMemoryLimit, nil
}

func Protect(pod *v1.Pod, pid int, thresholdRatio float64, scalaRatio float64, thresholdHitEventCh chan<- *MemoryProtect, limitHitEventCh chan<- *MemoryProtect) error {
	path := cgroups.PidPath(pid)
	control, err := cgroups.Load(cgroups.V1, path)
	if err != nil {
		logrus.Errorf("pid: %v cgroup load fail, %v", pid, err)
		return err
	}
	parentControl, err := cgroups.Load(cgroups.V1, parentPath(path))
	if err != nil {
		logrus.Errorf("pid: %v parent cgroup load fail, %v", pid, err)
		return err
	}
	stat, err := control.Stat(cgroups.IgnoreNotExist)
	if err != nil {
		return err
	}
	memLimit := stat.Memory.HierarchicalMemoryLimit
	memProtect := &MemoryProtect{
		Pod:                 pod,
		cgroup:              &control,
		parentCgroup:        &parentControl,
		ThresholdRatio:      thresholdRatio,
		ScalaRatio:          scalaRatio,
		MemoryThreshold:     int64(thresholdRatio * float64(memLimit)),
		OldMemLimit:         int64(memLimit),
		NewMemLimit:         int64(scalaRatio * float64(memLimit)),
		thresholdHitEventCh: thresholdHitEventCh,
		limitHitEventCh:     limitHitEventCh,
	}
	err = registerMemoryEvent(memProtect.MemoryThreshold, memProtect)
	if err != nil {
		logrus.Errorln("registerMemoryEvent, ", err)
		return err
	}
	logrus.Infof("pid: %v memory protector started", pid)
	return nil
}

func registerMemoryEvent(threshold int64, memProtect *MemoryProtect) error {
	if !notifierCache.add(string(memProtect.Pod.UID)) {
		return nil
	}

	cgroup := *memProtect.cgroup
	fd, err := cgroup.RegisterMemoryEvent(cgroups.MemoryThresholdEvent(uint64(threshold), false))
	if err != nil {
		return err
	}
	eventfd := os.NewFile(fd, "CgroupMemoryEvent")
	go func() {
		defer func() {
			_ = eventfd.Close()
		}()
		buf := make([]byte, 8)
		if _, err := eventfd.Read(buf); err != nil {
			return
		}
		if cgroup.State() == cgroups.Deleted {
			return
		}
		cgroup := *memProtect.cgroup
		logrus.Infof("dynamic provision memory limit from %v to %v", memProtect.OldMemLimit, memProtect.NewMemLimit)
		err := updateMemLimit(&cgroup, memProtect.NewMemLimit)
		if err != nil {
			logrus.Errorln("dynamic provision memory limit fail, ", err)
			return
		}
		parentCgroup := *memProtect.parentCgroup
		err = updateMemLimit(&parentCgroup, -1)
		if err != nil {
			logrus.Errorln("dynamic provision parent cgroup memory limit fail, ", err)
			return
		}
		memProtect.thresholdHitEventCh <- memProtect
		_ = registerMemoryLimitHitEvent(memProtect)
		notifierCache.del(string(memProtect.Pod.UID))
	}()
	return nil
}

func registerMemoryLimitHitEvent(memProtect *MemoryProtect) error {
	cgroup := *memProtect.cgroup
	fd, err := cgroup.RegisterMemoryEvent(cgroups.MemoryThresholdEvent(uint64(memProtect.OldMemLimit), false))
	if err != nil {
		return err
	}
	eventfd := os.NewFile(fd, "CgroupMemoryLimitHintEvent")
	go func() {
		defer func() {
			_ = eventfd.Close()
		}()
		buf := make([]byte, 8)
		if _, err := eventfd.Read(buf); err != nil {
			return
		}
		if cgroup.State() == cgroups.Deleted {
			return
		}
		memProtect.limitHitEventCh <- memProtect
	}()
	return nil
}
