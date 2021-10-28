// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/imdario/mergo"
	"github.com/mohae/deepcopy"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// Store is a central storage of metadata about workloads. A workload is any
// unit of work being done by a piece of software, like a process, a container,
// a kubernetes pod, or a task in any cloud provider.
type Store interface {
	Start(ctx context.Context)
	Subscribe(name string, filter *Filter) chan EventBundle
	Unsubscribe(ch chan EventBundle)
	GetContainer(id string) (*Container, error)
	GetKubernetesPod(id string) (*KubernetesPod, error)
	GetKubernetesPodForContainer(containerID string) (*KubernetesPod, error)
	GetECSTask(id string) (*ECSTask, error)
	Notify(events []CollectorEvent)
	Dump(verbose bool) WorkloadDumpResponse
}

// Kind is the kind of an entity.
type Kind string

// ContainerRuntime is the container runtime used by a container.
type ContainerRuntime string

// ECSLaunchType is the launch type of an ECS task.
type ECSLaunchType string

// EventType is the type of an event.
type EventType int

// List of enumerable constants for the types above.
const (
	KindContainer     Kind = "container"
	KindKubernetesPod Kind = "kubernetes_pod"
	KindECSTask       Kind = "ecs_task"

	ContainerRuntimeDocker     ContainerRuntime = "docker"
	ContainerRuntimeContainerd ContainerRuntime = "containerd"

	ECSLaunchTypeEC2     ECSLaunchType = "ec2"
	ECSLaunchTypeFargate ECSLaunchType = "fargate"

	EventTypeSet EventType = iota
	EventTypeUnset
)

// Entity is an item in the metadata store. It exists as an interface to avoid
// usage of interface{}.
type Entity interface {
	GetID() EntityID
	Merge(Entity) error
	DeepCopy() Entity
	String(verbose bool) string
}

// EntityID represents the ID of an Entity.
type EntityID struct {
	Kind Kind
	ID   string
}

// GetID satisfies the Entity interface for EntityID to allow a standalone
// EntityID to be passed in events of type EventTypeUnset without the need to
// build a full, concrete entity.
func (i EntityID) GetID() EntityID {
	return i
}

// Merge returns an error because EntityID is not expected to be merged with another Entity, because it's used
// as an identifier.
func (i EntityID) Merge(e Entity) error {
	return errors.New("cannot merge EntityID with another entity")
}

// DeepCopy returns a deep copy of EntityID.
func (i EntityID) DeepCopy() Entity {
	return i
}

// String returns a string representation of EntityID.
func (i EntityID) String(_ bool) string {
	var sb strings.Builder
	_, _ = sb.WriteString(fmt.Sprintln("Kind:", i.Kind, "ID:", i.ID))

	return sb.String()
}

var _ Entity = EntityID{}

// EntityMeta represents generic metadata about an Entity.
type EntityMeta struct {
	Name        string
	Namespace   string
	Annotations map[string]string
	Labels      map[string]string
}

// String returns a string representation of EntityMeta.
func (e EntityMeta) String(verbose bool) string {
	var sb strings.Builder
	_, _ = sb.WriteString(fmt.Sprintln("Name:", e.Name))
	_, _ = sb.WriteString(fmt.Sprintln("Namespace:", e.Namespace))

	if verbose {
		_, _ = sb.WriteString(fmt.Sprintln("Annotations:", mapToString(e.Annotations)))
		_, _ = sb.WriteString(fmt.Sprintln("Labels:", mapToString(e.Labels)))
	}

	return sb.String()
}

// ContainerImage is the an image used by a container.
type ContainerImage struct {
	ID        string
	RawName   string
	Name      string
	ShortName string
	Tag       string
}

// NewContainerImage builds a ContainerImage from an image name
func NewContainerImage(imageName string) (ContainerImage, error) {
	image := ContainerImage{
		RawName: imageName,
		Name:    imageName,
	}

	name, shortName, tag, err := containers.SplitImageName(imageName)
	if err != nil {
		return image, err
	}

	if tag == "" {
		tag = "latest"
	}

	image.Name = name
	image.ShortName = shortName
	image.Tag = tag

	return image, nil
}

// String returns a string representation of ContainerImage.
func (c ContainerImage) String(verbose bool) string {
	var sb strings.Builder
	_, _ = sb.WriteString(fmt.Sprintln("Name:", c.Name))
	_, _ = sb.WriteString(fmt.Sprintln("Tag:", c.Tag))

	if verbose {
		_, _ = sb.WriteString(fmt.Sprintln("ID:", c.ID))
		_, _ = sb.WriteString(fmt.Sprintln("Raw Name:", c.RawName))
		_, _ = sb.WriteString(fmt.Sprintln("Short Name:", c.ShortName))
	}

	return sb.String()
}

// ContainerState is the state of a container.
type ContainerState struct {
	Running    bool
	StartedAt  time.Time
	FinishedAt time.Time
}

// String returns a string representation of ContainerState.
func (c ContainerState) String(verbose bool) string {
	var sb strings.Builder
	_, _ = sb.WriteString(fmt.Sprintln("Running:", c.Running))

	if verbose {
		_, _ = sb.WriteString(fmt.Sprintln("Started At:", c.StartedAt))
		_, _ = sb.WriteString(fmt.Sprintln("Finished At:", c.FinishedAt))
	}

	return sb.String()
}

// ContainerPort is a port open in the container.
type ContainerPort struct {
	Name     string
	Port     int
	Protocol string
}

// String returns a string representation of ContainerPort.
func (c ContainerPort) String(verbose bool) string {
	var sb strings.Builder
	_, _ = sb.WriteString(fmt.Sprintln("Port:", c.Port))

	if verbose {
		_, _ = sb.WriteString(fmt.Sprintln("Name:", c.Name))
		_, _ = sb.WriteString(fmt.Sprintln("Protocol:", c.Protocol))
	}

	return sb.String()
}

// OrchestratorContainer is a reference to a Container with
// orchestrator-specific data attached to it.
type OrchestratorContainer struct {
	ID    string
	Name  string
	Image ContainerImage
}

// String returns a string representation of OrchestratorContainer.
func (o OrchestratorContainer) String(_ bool) string {
	var sb strings.Builder
	_, _ = sb.WriteString(fmt.Sprintln("Name:", o.Name, "ID:", o.ID))

	return sb.String()
}

// Container is a containerized workload.
type Container struct {
	EntityID
	EntityMeta
	EnvVars    map[string]string
	Hostname   string
	Image      ContainerImage
	NetworkIPs map[string]string
	PID        int
	Ports      []ContainerPort
	Runtime    ContainerRuntime
	State      ContainerState
}

// GetID returns the Container's EntityID.
func (c Container) GetID() EntityID {
	return c.EntityID
}

// Merge merges a Container with another. Returns an error if trying to merge
// with another kind.
func (c *Container) Merge(e Entity) error {
	cc, ok := e.(*Container)
	if !ok {
		return fmt.Errorf("cannot merge Container with different kind %T", e)
	}

	return mergo.Merge(c, cc)
}

// DeepCopy returns a deep copy of the container.
func (c Container) DeepCopy() Entity {
	cp := deepcopy.Copy(c).(Container)
	return &cp
}

// String returns a string representation of Container.
func (c Container) String(verbose bool) string {
	var sb strings.Builder

	_, _ = sb.WriteString(fmt.Sprintln("----------- Entity ID -----------"))
	_, _ = sb.WriteString(c.EntityID.String(verbose))

	_, _ = sb.WriteString(fmt.Sprintln("----------- Entity Meta -----------"))
	_, _ = sb.WriteString(c.EntityMeta.String(verbose))

	_, _ = sb.WriteString(fmt.Sprintln("----------- Image -----------"))
	_, _ = sb.WriteString(c.Image.String(verbose))

	_, _ = sb.WriteString(fmt.Sprintln("----------- Container Info -----------"))
	_, _ = sb.WriteString(fmt.Sprintln("Runtime:", c.Runtime))
	_, _ = sb.WriteString(c.State.String(verbose))

	if verbose {
		_, _ = sb.WriteString(fmt.Sprintln("Env Variables:", mapToString(c.EnvVars)))
		_, _ = sb.WriteString(fmt.Sprintln("Hostname:", c.Hostname))
		_, _ = sb.WriteString(fmt.Sprintln("Network IPs:", mapToString(c.NetworkIPs)))
		_, _ = sb.WriteString(fmt.Sprintln("PID:", c.PID))
	}

	if len(c.Ports) > 0 && verbose {
		_, _ = sb.WriteString(fmt.Sprintln("----------- Ports -----------"))
		for _, p := range c.Ports {
			_, _ = sb.WriteString(p.String(verbose))
		}
	}

	return sb.String()
}

var _ Entity = &Container{}

// KubernetesPod is a Kubernetes Pod.
type KubernetesPod struct {
	EntityID
	EntityMeta
	Owners                     []KubernetesPodOwner
	PersistentVolumeClaimNames []string
	Containers                 []OrchestratorContainer
	Ready                      bool
	Phase                      string
	IP                         string
	PriorityClass              string
	KubeServices               []string
	NamespaceLabels            map[string]string
}

// GetID returns the KubernetesPod's EntityID.
func (p KubernetesPod) GetID() EntityID {
	return p.EntityID
}

// Merge merges a KubernetesPod with another. Returns an error if trying to merge
// with another kind.
func (p *KubernetesPod) Merge(e Entity) error {
	pp, ok := e.(*KubernetesPod)
	if !ok {
		return fmt.Errorf("cannot merge KubernetesPod with different kind %T", e)
	}

	return mergo.Merge(p, pp)
}

// DeepCopy returns a deep copy of the pod.
func (p KubernetesPod) DeepCopy() Entity {
	cp := deepcopy.Copy(p).(KubernetesPod)
	return &cp
}

// String returns a string representation of KubernetesPod.
func (p KubernetesPod) String(verbose bool) string {
	var sb strings.Builder
	_, _ = sb.WriteString(fmt.Sprintln("----------- Entity ID -----------"))
	_, _ = sb.WriteString(p.EntityID.String(verbose))

	_, _ = sb.WriteString(fmt.Sprintln("----------- Entity Meta -----------"))
	_, _ = sb.WriteString(p.EntityMeta.String(verbose))

	if len(p.Owners) > 0 {
		_, _ = sb.WriteString(fmt.Sprintln("----------- Owners -----------"))
		for _, o := range p.Owners {
			_, _ = sb.WriteString(o.String(verbose))
		}
	}

	if len(p.Containers) > 0 {
		_, _ = sb.WriteString(fmt.Sprintln("----------- Containers -----------"))
		for _, c := range p.Containers {
			_, _ = sb.WriteString(c.String(verbose))
		}
	}

	_, _ = sb.WriteString(fmt.Sprintln("----------- Pod Info -----------"))
	_, _ = sb.WriteString(fmt.Sprintln("Ready:", p.Ready))
	_, _ = sb.WriteString(fmt.Sprintln("Phase:", p.Phase))
	_, _ = sb.WriteString(fmt.Sprintln("IP:", p.IP))

	if verbose {
		_, _ = sb.WriteString(fmt.Sprintln("Priority Class:", p.PriorityClass))
		_, _ = sb.WriteString(fmt.Sprintln("PVCs:", sliceToString(p.PersistentVolumeClaimNames)))
		_, _ = sb.WriteString(fmt.Sprintln("Kube Services:", sliceToString(p.KubeServices)))
		_, _ = sb.WriteString(fmt.Sprintln("Namespace Labels:", mapToString(p.NamespaceLabels)))
	}

	return sb.String()
}

var _ Entity = &KubernetesPod{}

// KubernetesPodOwner is extracted from a pod's owner references.
type KubernetesPodOwner struct {
	Kind string
	Name string
	ID   string
}

// String returns a string representation of KubernetesPodOwner.
func (o KubernetesPodOwner) String(verbose bool) string {
	var sb strings.Builder
	_, _ = sb.WriteString(fmt.Sprintln("Kind:", o.Kind, "Name:", o.Name))

	if verbose {
		_, _ = sb.WriteString(fmt.Sprintln("ID:", o.ID))

	}

	return sb.String()
}

// ECSTask is an ECS Task.
type ECSTask struct {
	EntityID
	EntityMeta
	Tags                  map[string]string
	ContainerInstanceTags map[string]string
	ClusterName           string
	Region                string
	AvailabilityZone      string
	Family                string
	Version               string
	LaunchType            ECSLaunchType
	Containers            []OrchestratorContainer
}

// GetID returns an ECSTasks's EntityID.
func (t ECSTask) GetID() EntityID {
	return t.EntityID
}

// Merge merges a ECSTask with another. Returns an error if trying to merge
// with another kind.
func (t *ECSTask) Merge(e Entity) error {
	tt, ok := e.(*ECSTask)
	if !ok {
		return fmt.Errorf("cannot merge ECSTask with different kind %T", e)
	}

	return mergo.Merge(t, tt)
}

// DeepCopy returns a deep copy of the task.
func (t ECSTask) DeepCopy() Entity {
	cp := deepcopy.Copy(t).(ECSTask)
	return &cp
}

// String returns a string representation of ECSTask.
func (t ECSTask) String(verbose bool) string {
	var sb strings.Builder
	_, _ = sb.WriteString(fmt.Sprintln("----------- Entity ID -----------"))
	_, _ = sb.WriteString(t.EntityID.String(verbose))

	_, _ = sb.WriteString(fmt.Sprintln("----------- Entity Meta -----------"))
	_, _ = sb.WriteString(t.EntityMeta.String(verbose))

	_, _ = sb.WriteString(fmt.Sprintln("----------- Containers -----------"))
	for _, c := range t.Containers {
		_, _ = sb.WriteString(c.String(verbose))
	}

	if verbose {
		_, _ = sb.WriteString(fmt.Sprintln("----------- Task Info -----------"))
		_, _ = sb.WriteString(fmt.Sprintln("Tags:", mapToString(t.Tags)))
		_, _ = sb.WriteString(fmt.Sprintln("Container Instance Tags:", mapToString(t.ContainerInstanceTags)))
		_, _ = sb.WriteString(fmt.Sprintln("Cluster Name:", t.ClusterName))
		_, _ = sb.WriteString(fmt.Sprintln("Region:", t.Region))
		_, _ = sb.WriteString(fmt.Sprintln("Availability Zone:", t.AvailabilityZone))
		_, _ = sb.WriteString(fmt.Sprintln("Family:", t.Family))
		_, _ = sb.WriteString(fmt.Sprintln("Version:", t.Version))
		_, _ = sb.WriteString(fmt.Sprintln("Launch Type:", t.LaunchType))
	}

	return sb.String()
}

var _ Entity = &ECSTask{}

// CollectorEvent is an event generated by a metadata collector, to be handled
// by the metadata store.
type CollectorEvent struct {
	Type   EventType
	Source string
	Entity Entity
}

// Event is an event generated by the metadata store, to be broadcasted to
// subscribers.
type Event struct {
	Type    EventType
	Sources []string
	Entity  Entity
}

// EventBundle is a collection of events, and a channel that needs to be closed
// when the receiving subscriber wants to unblock the notifier.
type EventBundle struct {
	Events []Event
	Ch     chan struct{}
}
