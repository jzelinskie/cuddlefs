package fs

import (
	"context"
	"os"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"

	"github.com/jzelinskie/cuddlefs/pkg/kubeutil"
	"github.com/jzelinskie/cuddlefs/pkg/strutil"
)

func New(cfg *rest.Config) (fs.FS, error) {
	client, err := kubeutil.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	return Root{client}, nil
}

type Root struct {
	client *kubeutil.Client
}

func (r Root) Root() (fs.Node, error) {
	return GroupsDir{r.client}, nil
}

type GroupsDir struct {
	client *kubeutil.Client
}

func (d GroupsDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Mode = os.ModeDir
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

	entries := make([]fuse.Dirent, 0, len(groupNames))
	for _, gvName := range groupNames {
		entries = append(entries, fuse.Dirent{Name: gvName, Type: fuse.DT_Dir})
	}
	return entries, nil
}

func (d GroupsDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
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
		return &ResourcesDir{d.client, versions[0]}, nil
	}
	if len(versions) > 0 {
		return &GroupVersionsDir{d.client, versions}, nil
	}

	return nil, fuse.ENOENT
}

type GroupVersionsDir struct {
	client        *kubeutil.Client
	groupVersions []*metav1.APIResourceList
}

func (d GroupVersionsDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Mode = os.ModeDir
	return nil
}

func (d GroupVersionsDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	entries := make([]fuse.Dirent, 0, len(d.groupVersions))
	for _, gv := range d.groupVersions {
		if len(gv.APIResources) == 0 {
			continue
		}
		_, version := kubeutil.SplitGroupVersion(gv.GroupVersion)
		entries = append(entries, fuse.Dirent{Name: version, Type: fuse.DT_Dir})
	}
	return entries, nil
}

func (d GroupVersionsDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	for _, gv := range d.groupVersions {
		if len(gv.APIResources) == 0 {
			continue
		}
		_, version := kubeutil.SplitGroupVersion(gv.GroupVersion)
		if name == version {
			return &ResourcesDir{d.client, gv}, nil
		}
	}

	return nil, fuse.ENOENT
}

type ResourcesDir struct {
	client       *kubeutil.Client
	resourceList *metav1.APIResourceList
}

func (d ResourcesDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Mode = os.ModeDir
	return nil
}

func (d ResourcesDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	entries := make([]fuse.Dirent, 0, len(d.resourceList.APIResources))
	for _, resource := range d.resourceList.APIResources {
		entries = append(entries, fuse.Dirent{Name: resource.Name, Type: fuse.DT_Dir})
	}
	return entries, nil
}

func (d ResourcesDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	for _, resource := range d.resourceList.APIResources {
		if resource.Name == name {
			group, version := kubeutil.SplitGroupVersion(d.resourceList.GroupVersion)
			gvk := schema.GroupVersionKind{
				Group:   group,
				Version: version,
				Kind:    resource.Kind,
			}
			return &ResourceDir{d.client, "", gvk}, nil
		}
	}

	return nil, fuse.ENOENT
}

type ResourceDir struct {
	client    *kubeutil.Client
	namespace string
	gvk       schema.GroupVersionKind
}

func (d ResourceDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Mode = os.ModeDir
	return nil
}

func (d ResourceDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	u := &unstructured.UnstructuredList{}
	u.SetGroupVersionKind(d.gvk)

	err := d.client.List(ctx, nil, u)
	if err != nil {
		return nil, err
	}

	entries := make([]fuse.Dirent, 0, len(u.Items))
	for _, item := range u.Items {
		metadata := item.Object["metadata"].(map[string]interface{})
		name := metadata["name"].(string)
		entries = append(entries, fuse.Dirent{Name: name, Type: fuse.DT_Dir})
	}

	return entries, nil
}

func (d ResourceDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
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
			return ObjectDir{}, nil
		}
	}

	return nil, fuse.ENOENT
}

type ObjectDir struct{}

func (d ObjectDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Mode = os.ModeDir
	return nil
}

func (d ObjectDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	entries := make([]fuse.Dirent, 0, 2)
	entries = append(entries, fuse.Dirent{Name: "yaml", Type: fuse.DT_File})
	entries = append(entries, fuse.Dirent{Name: "json", Type: fuse.DT_File})
	return entries, nil
}

func (d ObjectDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	return nil, fuse.ENOENT
}
