package fs

import (
	"context"
	"encoding/json"
	"os"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/jzelinskie/stringz"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"

	"github.com/jzelinskie/cuddlefs/pkg/kubeutil"
)

func New(logger *zap.Logger, cfg *rest.Config) (fs.FS, error) {
	client, err := kubeutil.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	return RootDir{logger, client}, nil
}

type RootDir struct {
	logger *zap.Logger
	client *kubeutil.Client
}

func (d RootDir) Root() (fs.Node, error) {
	return ViewsDir{d.logger, d.client}, nil
}

type ViewsDir struct {
	logger *zap.Logger
	client *kubeutil.Client
}

func (d ViewsDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Mode = os.ModeDir
	d.logger.Debug("attr on views dir",
		zap.Uint32("mode", uint32(attr.Mode)),
	)
	return nil
}

func (d ViewsDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	views := []string{"by-gvk"}
	d.logger.Debug("readdir on views dir",
		zap.Strings("entries", views),
	)
	return StringsToDirents(views), nil
}

func (d ViewsDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	d.logger.Debug("lookup on views dir",
		zap.String("name", name),
	)

	if name == "by-gvk" {
		return GroupsDir{d.logger, d.client}, nil
	}

	return nil, fuse.ENOENT
}

type GroupsDir struct {
	logger *zap.Logger
	client *kubeutil.Client
}

func (d GroupsDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Mode = os.ModeDir
	d.logger.Debug("attr on groups dir",
		zap.Uint32("mode", uint32(attr.Mode)),
	)
	return nil
}

func (d GroupsDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	groupVersions, err := d.client.ServerPreferredResources()
	if err != nil {
		return nil, err
	}

	groupNames := make([]string, 0)
	for _, gv := range groupVersions {
		group, version := kubeutil.SplitGroupVersion(gv.GroupVersion)
		if group == "" {
			group = version
		}
		groupNames = append(groupNames, group)
	}
	groupNames = stringz.Dedup(groupNames)

	d.logger.Debug("readdir on groups dir",
		zap.Strings("entries", groupNames),
	)

	return StringsToDirents(groupNames), nil
}

func (d GroupsDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	d.logger.Debug("lookup on groups dir",
		zap.String("name", name),
	)

	groupVersions, err := d.client.ServerPreferredResources()
	if err != nil {
		return nil, err
	}

	// Get all the groupVersions in the same group.
	versions := make([]*metav1.APIResourceList, 0)
	for _, gv := range groupVersions {
		group, version := kubeutil.SplitGroupVersion(gv.GroupVersion)
		if name == group {
			versions = append(versions, gv)
		} else if name == "v1" && version == "v1" && group == "" {
			versions = append(versions, gv)
		}
	}

	if len(versions) == 1 {
		return &ResourcesDir{d.logger, d.client, versions[0]}, nil
	}
	if len(versions) > 0 {
		return &GroupVersionsDir{d.logger, d.client, versions}, nil
	}

	return nil, fuse.ENOENT
}

type GroupVersionsDir struct {
	logger        *zap.Logger
	client        *kubeutil.Client
	groupVersions []*metav1.APIResourceList
}

func (d GroupVersionsDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Mode = os.ModeDir
	d.logger.Debug("attr on gvk dir",
		zap.Uint32("mode", uint32(attr.Mode)),
	)
	return nil
}

func (d GroupVersionsDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	versions := make([]string, 0, len(d.groupVersions))
	for _, gv := range d.groupVersions {
		if len(gv.APIResources) == 0 {
			continue
		}
		_, version := kubeutil.SplitGroupVersion(gv.GroupVersion)
		versions = append(versions, version)
	}

	d.logger.Debug("readdir on gvk dir",
		zap.Strings("entries", versions),
	)
	return StringsToDirents(versions), nil
}

func (d GroupVersionsDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	d.logger.Debug("lookup on gvk dir",
		zap.String("name", name),
	)

	for _, gv := range d.groupVersions {
		if len(gv.APIResources) == 0 {
			continue
		}
		_, version := kubeutil.SplitGroupVersion(gv.GroupVersion)
		if name == version {
			return &ResourcesDir{d.logger, d.client, gv}, nil
		}
	}

	return nil, fuse.ENOENT
}

type ResourcesDir struct {
	logger       *zap.Logger
	client       *kubeutil.Client
	resourceList *metav1.APIResourceList
}

func (d ResourcesDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Mode = os.ModeDir
	d.logger.Debug("attr on resources dir",
		zap.Uint32("mode", uint32(attr.Mode)),
	)
	return nil
}

func (d ResourcesDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	names := make([]string, 0, len(d.resourceList.APIResources))
	for _, resource := range d.resourceList.APIResources {
		names = append(names, resource.Name)
	}

	d.logger.Debug("readdir on resources dir",
		zap.Strings("entries", names),
	)

	return StringsToDirents(names), nil
}

func (d ResourcesDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	d.logger.Debug("lookup on resources dir",
		zap.String("name", name),
	)

	for _, resource := range d.resourceList.APIResources {
		if resource.Name == name {
			group, version := kubeutil.SplitGroupVersion(d.resourceList.GroupVersion)
			gvk := schema.GroupVersionKind{
				Group:   group,
				Version: version,
				Kind:    resource.Kind,
			}

			ulist := &unstructured.UnstructuredList{}
			ulist.SetGroupVersionKind(gvk)

			err := d.client.List(ctx, nil, ulist)
			if err != nil {
				return nil, err
			}

			if resource.Namespaced {
				return &ResourceNamespacesDir{d.logger, d.client, ulist}, nil
			}
			return &ResourceDir{d.logger, d.client, "", ulist}, nil
		}
	}

	return nil, fuse.ENOENT
}

type ResourceNamespacesDir struct {
	logger *zap.Logger
	client *kubeutil.Client
	ulist  *unstructured.UnstructuredList
}

func (d ResourceNamespacesDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Mode = os.ModeDir
	d.logger.Debug("attr on resource namespaces dir",
		zap.Uint32("mode", uint32(attr.Mode)),
	)
	return nil
}

func (d ResourceNamespacesDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	entries := kubeutil.Namespaces(d.ulist)
	d.logger.Debug("readdir on resource dir",
		zap.Strings("entries", entries),
	)
	return StringsToDirents(entries), nil
}

func (d ResourceNamespacesDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	for _, namespace := range kubeutil.Namespaces(d.ulist) {
		if name == namespace {
			return &ResourceDir{d.logger, d.client, namespace, d.ulist}, nil
		}
	}
	return nil, fuse.ENOENT
}

type ResourceDir struct {
	logger    *zap.Logger
	client    *kubeutil.Client
	namespace string
	ulist     *unstructured.UnstructuredList
}

func (d ResourceDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Mode = os.ModeDir
	d.logger.Debug("attr on resource dir",
		zap.Uint32("mode", uint32(attr.Mode)),
	)
	return nil
}

func (d ResourceDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	entries := make([]string, 0, len(d.ulist.Items))
	for _, item := range d.ulist.Items {
		if d.namespace == "" || d.namespace == item.GetNamespace() {
			entries = append(entries, item.GetName())
		}
	}

	d.logger.Debug("readdir on resource dir",
		zap.Strings("entries", entries),
	)

	return StringsToDirents(entries), nil
}

func (d ResourceDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	d.logger.Debug("lookup on resource dir",
		zap.String("name", name),
	)

	for _, item := range d.ulist.Items {
		if name == item.GetName() {
			// See if there's a better type than the generic Object type.
			gvk := item.GroupVersionKind()
			if fn, ok := specificDirs[gvk]; ok {
				d.logger.Debug("found a more specific directory type",
					zap.Reflect("gvk", gvk),
				)

				return fn(d.logger, d.client, &item), nil
			}
			return ObjectDir{d.logger, d.client, &item}, nil
		}
	}

	return nil, fuse.ENOENT
}

type ObjectDir struct {
	logger *zap.Logger
	client *kubeutil.Client
	u      *unstructured.Unstructured
}

func (d ObjectDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Mode = os.ModeDir
	d.logger.Debug("attr on object dir",
		zap.Uint32("mode", uint32(attr.Mode)),
		zap.Uint64("size", attr.Size),
	)
	return nil
}

func (d ObjectDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	files := []string{"yaml", "json"}
	d.logger.Debug("readdir on object dir",
		zap.Strings("entries", files),
	)

	return StringsToDirents(files), nil
}

func (d ObjectDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	d.logger.Debug("lookup on object dir",
		zap.String("name", name),
	)

	if name == "json" {
		data, err := json.MarshalIndent(d.u, "", "  ")
		d.logger.Debug("serialized JSON",
			zap.String("JSON", string(data)),
			zap.Error(err),
		)
		return &File{d.logger, data}, err
	}

	if name == "yaml" {
		data, err := yaml.Marshal(d.u)
		d.logger.Debug("serialized YAML",
			zap.String("YAML", string(data)),
			zap.Error(err),
		)
		return &File{d.logger, data}, err
	}

	return nil, fuse.ENOENT
}

type File struct {
	logger   *zap.Logger
	contents []byte
}

func (f *File) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Size = uint64(len(f.contents))
	f.logger.Debug("attr on file",
		zap.Uint32("mode", uint32(attr.Mode)),
		zap.Uint64("size", attr.Size),
	)
	return nil
}

func (f *File) ReadAll(ctx context.Context) ([]byte, error) {
	f.logger.Debug("read on file")
	return f.contents, nil
}

type SpecificObjectConstructor func(*zap.Logger, *kubeutil.Client, *unstructured.Unstructured) SpecificObjectDir

type SpecificObjectDir interface {
	fs.Node
	fs.HandleReadDirAller
}

var specificDirs = map[schema.GroupVersionKind]SpecificObjectConstructor{
	{Group: "", Version: "v1", Kind: "ConfigMap"}: NewConfigMapDir,
}

func NewConfigMapDir(logger *zap.Logger, client *kubeutil.Client, u *unstructured.Unstructured) SpecificObjectDir {
	cm, err := kubeutil.UnstructuredToConfigMap(u)
	if err != nil {
		panic(err)
	}
	return ConfigMapDir{logger, client, cm}
}

type ConfigMapDir struct {
	logger *zap.Logger
	client *kubeutil.Client
	cm     *corev1.ConfigMap
}

func (d ConfigMapDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Mode = os.ModeDir
	d.logger.Debug("attr on configmap dir",
		zap.Uint32("mode", uint32(attr.Mode)),
	)

	return nil
}

func (d ConfigMapDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	return StringsToDirents([]string{"data", "yaml", "json"}), nil
}

func (d ConfigMapDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	d.logger.Debug("lookup on configmap dir", zap.String("name", name))

	if name == "data" {
		return &DataDir{d.logger, d.cm.Data}, nil
	}

	if name == "json" {
		data, err := json.MarshalIndent(d.cm, "", "  ")
		d.logger.Debug("serialized JSON",
			zap.String("JSON", string(data)),
			zap.Error(err),
		)
		return &File{d.logger, data}, err
	}

	if name == "yaml" {
		data, err := yaml.Marshal(d.cm)
		d.logger.Debug("serialized YAML",
			zap.String("YAML", string(data)),
			zap.Error(err),
		)
		return &File{d.logger, data}, err
	}

	return nil, fuse.ENOENT
}

type DataDir struct {
	logger *zap.Logger
	data   map[string]string
}

func (d DataDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Mode = os.ModeDir
	d.logger.Debug("attr on data dir",
		zap.Uint32("mode", uint32(attr.Mode)),
	)

	return nil
}

func (d DataDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	names := make([]string, 0, len(d.data))
	for key := range d.data {
		names = append(names, key)
	}

	d.logger.Debug("readdir on data dir",
		zap.Strings("entries", names),
	)

	return StringsToDirents(names), nil
}

func (d DataDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	d.logger.Debug("lookup on data dir", zap.String("name", name))

	for key := range d.data {
		if name == key {
			return &File{d.logger, []byte(d.data[key])}, nil
		}
	}

	return nil, fuse.ENOENT
}
