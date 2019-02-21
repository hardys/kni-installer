package bootkube

import (
	"os"
	"path/filepath"

	"github.com/openshift-metalkube/kni-installer/pkg/asset"
	"github.com/openshift-metalkube/kni-installer/pkg/asset/templates/content"
)

const (
	etcdCAConfigMapFileName = "etcd-ca-bundle-configmap.yaml.template"
)

var _ asset.WritableAsset = (*EtcdCAConfigMap)(nil)

// EtcdCAConfigMap is an asset for the etcd ca
type EtcdCAConfigMap struct {
	FileList []*asset.File
}

// Dependencies returns all of the dependencies directly needed by the asset
func (t *EtcdCAConfigMap) Dependencies() []asset.Asset {
	return []asset.Asset{}
}

// Name returns the human-friendly name of the asset.
func (t *EtcdCAConfigMap) Name() string {
	return "EtcdCAConfigMap"
}

// Generate generates the actual files by this asset
func (t *EtcdCAConfigMap) Generate(parents asset.Parents) error {
	fileName := etcdCAConfigMapFileName
	data, err := content.GetBootkubeTemplate(fileName)
	if err != nil {
		return err
	}
	t.FileList = []*asset.File{
		{
			Filename: filepath.Join(content.TemplateDir, fileName),
			Data:     []byte(data),
		},
	}
	return nil
}

// Files returns the files generated by the asset.
func (t *EtcdCAConfigMap) Files() []*asset.File {
	return t.FileList
}

// Load returns the asset from disk.
func (t *EtcdCAConfigMap) Load(f asset.FileFetcher) (bool, error) {
	file, err := f.FetchByName(filepath.Join(content.TemplateDir, etcdCAConfigMapFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	t.FileList = []*asset.File{file}
	return true, nil
}
