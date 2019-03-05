package main

import (
	"context"
	"os"
	"path/filepath"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"

	"github.com/jzelinskie/cuddlefs/pkg/kubeutil"
	"github.com/jzelinskie/cuddlefs/pkg/strutil"
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "cuddlefs [flags]",
		Short: "k8s filesystem",
		Long:  "cuddlefs is a userspace filesystem for Kubernetes",
		RunE:  rootRunFunc,
	}

	rootCmd.Flags().String("mount", "./cluster", "path where the filesystem will be mounted")
	rootCmd.Flags().String("kubeconfig", filepath.Join(os.ExpandEnv("$HOME"), ".kube", "config"), "path to kubeconfig")

	rootCmd.Execute()
}

func mustGetString(cmd *cobra.Command, flag string) string {
	value, err := cmd.Flags().GetString(flag)
	if err != nil {
		panic(err)
	}
	return value
}

func rootRunFunc(cmd *cobra.Command, args []string) error {
	config, err := clientcmd.BuildConfigFromFlags("", mustGetString(cmd, "kubeconfig"))
	if err != nil {
		return err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	c, err := fuse.Mount(
		mustGetString(cmd, "mount"),
		fuse.FSName("cuddlefs"),
		fuse.Subtype("cuddlefs"),
		fuse.LocalVolume(),
		fuse.VolumeName("Kubernetes"),
	)
	if err != nil {
		return err
	}
	defer c.Close()

	err = fs.Serve(c, FS{clientset})
	if err != nil {
		return err
	}

	<-c.Ready
	if err := c.MountError; err != nil {
		return err
	}

	return nil
}

type Resource struct {
	clientset *kubernetes.Clientset
	namespace string
	resource  string
	name      string
}

func (r Resource) getYAML() []byte {
	// TODO(jzelinskie): ya know, do more than just pods
	pod, _ := r.clientset.CoreV1().Pods(r.namespace).Get(r.name, metav1.GetOptions{})
	podYAML, _ := yaml.Marshal(pod)
	return podYAML
}

func (r Resource) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Size = uint64(len(r.getYAML()))
	return nil
}

func (r Resource) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	contents := r.getYAML()
	resp.Data = contents[int(req.Offset) : int(req.Offset)+int(req.Size)]
	return nil
}

type Pods struct {
	clientset *kubernetes.Clientset
	namespace string
}

func (Pods) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Mode = os.ModeDir
	return nil
}

func (p Pods) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	podNames, err := kubeutil.PodNames(p.clientset.CoreV1().Pods(p.namespace).List(metav1.ListOptions{}))
	if err != nil {
		return nil, err
	}

	entries := make([]fuse.Dirent, 0, len(podNames))
	for _, name := range podNames {
		entries = append(entries, fuse.Dirent{Name: name, Type: fuse.DT_File})
	}
	return entries, nil
}

func (p Pods) Lookup(ctx context.Context, name string) (fs.Node, error) {
	podNames, err := kubeutil.PodNames(p.clientset.CoreV1().Pods(p.namespace).List(metav1.ListOptions{}))
	if err != nil {
		return nil, err
	}

	if strutil.Contains(podNames, name) {
		return Resource{p.clientset, p.namespace, "pod", name}, nil
	}
	return nil, fuse.ENOENT
}

type FS struct {
	clientset *kubernetes.Clientset
}

func (fs FS) Root() (fs.Node, error) {
	return Namespaces{fs.clientset}, nil
}

type Namespaces struct {
	clientset *kubernetes.Clientset
}

func (n Namespaces) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Mode = os.ModeDir
	return nil
}

func (n Namespaces) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	nsNames, err := kubeutil.NamespaceNames(n.clientset.CoreV1().Namespaces().List(metav1.ListOptions{}))
	if err != nil {
		return nil, err
	}

	entries := make([]fuse.Dirent, 0, len(nsNames))
	for _, name := range nsNames {
		entries = append(entries, fuse.Dirent{Name: name, Type: fuse.DT_Dir})
	}
	return entries, nil
}

func (n Namespaces) Lookup(ctx context.Context, name string) (fs.Node, error) {
	nsNames, err := kubeutil.NamespaceNames(n.clientset.CoreV1().Namespaces().List(metav1.ListOptions{}))
	if err != nil {
		return nil, err
	}

	if strutil.Contains(nsNames, name) {
		return NamespacedResources{n.clientset, name}, nil
	}
	return nil, fuse.ENOENT
}

type NamespacedResources struct {
	clientset *kubernetes.Clientset
	namespace string
}

func (r NamespacedResources) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Mode = os.ModeDir
	return nil
}

func (r NamespacedResources) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	resourceNames, err := kubeutil.NamespacedResourceNames(r.clientset.ServerResources())
	if err != nil {
		return nil, err
	}

	entries := make([]fuse.Dirent, 0, len(resourceNames))
	for _, resource := range resourceNames {
		entries = append(entries, fuse.Dirent{Name: resource, Type: fuse.DT_Dir})
	}
	return entries, nil
}

func (r NamespacedResources) Lookup(ctx context.Context, name string) (fs.Node, error) {
	resourceNames, err := kubeutil.NamespacedResourceNames(r.clientset.ServerResources())
	if err != nil {
		return nil, err
	}

	if strutil.Contains(resourceNames, name) {
		// TODO(jzelinskie): everything is not a pod ffs
		return Pods{r.clientset, r.namespace}, nil
	}

	return nil, fuse.ENOENT
}
