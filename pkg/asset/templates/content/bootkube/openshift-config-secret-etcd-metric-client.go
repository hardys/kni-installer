package bootkube

import (
	"os"
	"path/filepath"

	"github.com/openshift-metalkube/kni-installer/pkg/asset"
	"github.com/openshift-metalkube/kni-installer/pkg/asset/templates/content"
)

const (
	openshiftConfigSecretEtcdMetricClientFileName = "openshift-config-secret-etcd-metric-client.yaml.template"
)

var _ asset.WritableAsset = (*OpenshiftConfigSecretEtcdMetricClient)(nil)

// OpenshiftConfigSecretEtcdMetricClient is the constant to represent contents of openshift-config-secret-etcd-metric-client.yaml.template file.
type OpenshiftConfigSecretEtcdMetricClient struct {
	FileList []*asset.File
}

// Dependencies returns all of the dependencies directly needed by the asset
func (t *OpenshiftConfigSecretEtcdMetricClient) Dependencies() []asset.Asset {
	return []asset.Asset{}
}

// Name returns the human-friendly name of the asset.
func (t *OpenshiftConfigSecretEtcdMetricClient) Name() string {
	return "OpenshiftConfigSecretEtcdMetricClient"
}

// Generate generates the actual files by this asset
func (t *OpenshiftConfigSecretEtcdMetricClient) Generate(parents asset.Parents) error {
	fileName := openshiftConfigSecretEtcdMetricClientFileName
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
func (t *OpenshiftConfigSecretEtcdMetricClient) Files() []*asset.File {
	return t.FileList
}

// Load returns the asset from disk.
func (t *OpenshiftConfigSecretEtcdMetricClient) Load(f asset.FileFetcher) (bool, error) {
	file, err := f.FetchByName(filepath.Join(content.TemplateDir, openshiftConfigSecretEtcdMetricClientFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	t.FileList = []*asset.File{file}
	return true, nil
}