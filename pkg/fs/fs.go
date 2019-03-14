package fs

import (
	"context"
	"encoding/json"
	"os"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"

	"github.com/jzelinskie/cuddlefs/pkg/kubeutil"
	"github.com/jzelinskie/cuddlefs/pkg/strutil"
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
	return GroupsDir{d.logger, d.client}, nil
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
	groupNames = strutil.Dedup(groupNames)

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
			return &ResourceDir{d.logger, d.client, "", gvk}, nil
		}
	}

	return nil, fuse.ENOENT
}

type ResourceDir struct {
	logger    *zap.Logger
	client    *kubeutil.Client
	namespace string
	gvk       schema.GroupVersionKind
}

func (d ResourceDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Mode = os.ModeDir
	d.logger.Debug("attr on resource dir",
		zap.Uint32("mode", uint32(attr.Mode)),
	)
	return nil
}

func (d ResourceDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	u := &unstructured.UnstructuredList{}
	u.SetGroupVersionKind(d.gvk)

	err := d.client.List(ctx, nil, u)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(u.Items))
	for _, item := range u.Items {
		metadata := item.Object["metadata"].(map[string]interface{})
		names = append(names, metadata["name"].(string))
	}

	d.logger.Debug("readdir on resource dir",
		zap.Strings("entries", names),
	)

	return StringsToDirents(names), nil
}

func (d ResourceDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	d.logger.Debug("lookup on resource dir",
		zap.String("name", name),
		zap.String("group", d.gvk.Group),
		zap.String("version", d.gvk.Version),
		zap.String("kind", d.gvk.Kind),
	)

	u := &unstructured.UnstructuredList{}
	u.SetGroupVersionKind(d.gvk)

	err := d.client.List(ctx, nil, u)
	if err != nil {
		return nil, err
	}

	for _, item := range u.Items {
		metadata := item.Object["metadata"].(map[string]interface{})
		itemName := metadata["name"].(string)
		if name == itemName {
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
	entries := make([]fuse.Dirent, 0, 2)
	entries = append(entries, fuse.Dirent{Name: "yaml", Type: fuse.DT_File})
	entries = append(entries, fuse.Dirent{Name: "json", Type: fuse.DT_File})

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
		return &ObjectFile{d.logger, data}, err
	}

	if name == "yaml" {
		data, err := yaml.Marshal(d.u)
		d.logger.Debug("serialized YAML",
			zap.String("YAML", string(data)),
			zap.Error(err),
		)
		return &ObjectFile{d.logger, data}, err
	}

	return nil, fuse.ENOENT
}

type ObjectFile struct {
	logger   *zap.Logger
	contents []byte
}

func (f *ObjectFile) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Size = uint64(len(f.contents))
	f.logger.Debug("attr on object file",
		zap.Uint32("mode", uint32(attr.Mode)),
		zap.Uint64("size", attr.Size),
	)
	return nil
}

func (f *ObjectFile) ReadAll(ctx context.Context) ([]byte, error) {
	return f.contents, nil
}
