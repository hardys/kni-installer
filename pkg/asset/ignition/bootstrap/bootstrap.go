package bootstrap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/coreos/ignition/config/util"
	igntypes "github.com/coreos/ignition/config/v2_2/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/openshift-metalkube/kni-installer/data"
	"github.com/openshift-metalkube/kni-installer/pkg/asset"
	"github.com/openshift-metalkube/kni-installer/pkg/asset/ignition"
	"github.com/openshift-metalkube/kni-installer/pkg/asset/installconfig"
	"github.com/openshift-metalkube/kni-installer/pkg/asset/kubeconfig"
	"github.com/openshift-metalkube/kni-installer/pkg/asset/machines"
	"github.com/openshift-metalkube/kni-installer/pkg/asset/manifests"
	"github.com/openshift-metalkube/kni-installer/pkg/asset/tls"
	"github.com/openshift-metalkube/kni-installer/pkg/types"
)

const (
	rootDir              = "/opt/openshift"
	bootstrapIgnFilename = "bootstrap.ign"
	etcdCertSignerImage  = "quay.io/coreos/kube-etcd-signer-server:678cc8e6841e2121ebfdb6e2db568fce290b67d6"
	ignitionUser         = "core"
)

var (
	defaultReleaseImage = "registry.svc.ci.openshift.org/openshift/origin-release:v4.0"
)

// bootstrapTemplateData is the data to use to replace values in bootstrap
// template files.
type bootstrapTemplateData struct {
	EtcdCertSignerImage string
	EtcdCluster         string
	PullSecret          string
	ReleaseImage        string
}

// Bootstrap is an asset that generates the ignition config for bootstrap nodes.
type Bootstrap struct {
	Config *igntypes.Config
	File   *asset.File
}

var _ asset.WritableAsset = (*Bootstrap)(nil)

// Dependencies returns the assets on which the Bootstrap asset depends.
func (a *Bootstrap) Dependencies() []asset.Asset {
	return []asset.Asset{
		&installconfig.InstallConfig{},
		&kubeconfig.AdminClient{},
		&kubeconfig.Kubelet{},
		&machines.Master{},
		&manifests.Manifests{},
		&manifests.Openshift{},
		&tls.AdminKubeConfigCABundle{},
		&tls.AggregatorCA{},
		&tls.AggregatorCABundle{},
		&tls.AggregatorClientCertKey{},
		&tls.AggregatorSignerCertKey{},
		&tls.APIServerCertKey{},
		&tls.APIServerProxyCertKey{},
		&tls.EtcdCA{},
		&tls.EtcdCABundle{},
		&tls.EtcdClientCertKey{},
		&tls.EtcdMetricsCABundle{},
		&tls.EtcdMetricsSignerClientCertKey{},
		&tls.EtcdMetricsSignerServerCertKey{},
		&tls.EtcdSignerCertKey{},
		&tls.EtcdSignerClientCertKey{},
		&tls.JournalCertKey{},
		&tls.KubeAPIServerLBCABundle{},
		&tls.KubeAPIServerLBServerCertKey{},
		&tls.KubeAPIServerLBSignerCertKey{},
		&tls.KubeAPIServerLocalhostCABundle{},
		&tls.KubeAPIServerLocalhostServerCertKey{},
		&tls.KubeAPIServerLocalhostSignerCertKey{},
		&tls.KubeAPIServerServiceNetworkCABundle{},
		&tls.KubeAPIServerServiceNetworkServerCertKey{},
		&tls.KubeAPIServerServiceNetworkSignerCertKey{},
		&tls.KubeAPIServerCompleteCABundle{},
		&tls.KubeAPIServerCompleteClientCABundle{},
		&tls.KubeAPIServerToKubeletCABundle{},
		&tls.KubeAPIServerToKubeletClientCertKey{},
		&tls.KubeAPIServerToKubeletSignerCertKey{},
		&tls.KubeCA{},
		&tls.KubeControlPlaneCABundle{},
		&tls.KubeControlPlaneKubeControllerManagerClientCertKey{},
		&tls.KubeControlPlaneKubeSchedulerClientCertKey{},
		&tls.KubeControlPlaneSignerCertKey{},
		&tls.KubeletBootstrapCABundle{},
		&tls.KubeletClientCABundle{},
		&tls.KubeletClientCertKey{},
		&tls.KubeletCSRSignerCertKey{},
		&tls.KubeletServingCABundle{},
		&tls.MCSCertKey{},
		&tls.RootCA{},
		&tls.ServiceAccountKeyPair{},
	}
}

// Generate generates the ignition config for the Bootstrap asset.
func (a *Bootstrap) Generate(dependencies asset.Parents) error {
	installConfig := &installconfig.InstallConfig{}
	dependencies.Get(installConfig)

	templateData, err := a.getTemplateData(installConfig.Config)
	if err != nil {
		return errors.Wrap(err, "failed to get bootstrap templates")
	}

	a.Config = &igntypes.Config{
		Ignition: igntypes.Ignition{
			Version: igntypes.MaxVersion.String(),
		},
	}

	err = a.addStorageFiles("/", "bootstrap/files", templateData)
	if err != nil {
		return err
	}
	err = a.addSystemdUnits("bootstrap/systemd/units", templateData)
	if err != nil {
		return err
	}
	a.addParentFiles(dependencies)

	a.Config.Passwd.Users = append(
		a.Config.Passwd.Users,
		igntypes.PasswdUser{Name: "core", SSHAuthorizedKeys: []igntypes.SSHAuthorizedKey{igntypes.SSHAuthorizedKey(installConfig.Config.SSHKey)}},
	)

	data, err := json.Marshal(a.Config)
	if err != nil {
		return errors.Wrap(err, "failed to Marshal Ignition config")
	}
	a.File = &asset.File{
		Filename: bootstrapIgnFilename,
		Data:     data,
	}

	return nil
}

// Name returns the human-friendly name of the asset.
func (a *Bootstrap) Name() string {
	return "Bootstrap Ignition Config"
}

// Files returns the files generated by the asset.
func (a *Bootstrap) Files() []*asset.File {
	if a.File != nil {
		return []*asset.File{a.File}
	}
	return []*asset.File{}
}

// getTemplateData returns the data to use to execute bootstrap templates.
func (a *Bootstrap) getTemplateData(installConfig *types.InstallConfig) (*bootstrapTemplateData, error) {
	etcdEndpoints := make([]string, *installConfig.ControlPlane.Replicas)
	for i := range etcdEndpoints {
		etcdEndpoints[i] = fmt.Sprintf("https://etcd-%d.%s:2379", i, installConfig.ClusterDomain())
	}

	releaseImage := defaultReleaseImage
	if ri, ok := os.LookupEnv("OPENSHIFT_INSTALL_RELEASE_IMAGE_OVERRIDE"); ok && ri != "" {
		logrus.Warn("Found override for ReleaseImage. Please be warned, this is not advised")
		releaseImage = ri
	}

	return &bootstrapTemplateData{
		EtcdCertSignerImage: etcdCertSignerImage,
		PullSecret:          installConfig.PullSecret,
		ReleaseImage:        releaseImage,
		EtcdCluster:         strings.Join(etcdEndpoints, ","),
	}, nil
}

func (a *Bootstrap) addStorageFiles(base string, uri string, templateData *bootstrapTemplateData) (err error) {
	file, err := data.Assets.Open(uri)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	if info.IsDir() {
		children, err := file.Readdir(0)
		if err != nil {
			return err
		}
		if err = file.Close(); err != nil {
			return err
		}

		for _, childInfo := range children {
			name := childInfo.Name()
			err = a.addStorageFiles(path.Join(base, name), path.Join(uri, name), templateData)
			if err != nil {
				return err
			}
		}
		return nil
	}

	name := info.Name()
	_, data, err := readFile(name, file, templateData)
	if err != nil {
		return err
	}

	filename := path.Base(uri)

	var mode int
	appendToFile := false
	if path.Base(path.Dir(uri)) == "bin" {
		mode = 0555
	} else if filename == "motd" {
		mode = 0644
		appendToFile = true
	} else {
		mode = 0600
	}
	ign := ignition.FileFromBytes(strings.TrimSuffix(base, ".template"), "root", mode, data)
	ign.Append = appendToFile
	a.Config.Storage.Files = append(a.Config.Storage.Files, ign)

	return nil
}

func (a *Bootstrap) addSystemdUnits(uri string, templateData *bootstrapTemplateData) (err error) {
	enabled := map[string]struct{}{
		"progress.service":                {},
		"kubelet.service":                 {},
		"keepalived.service":              {},
		"systemd-journal-gatewayd.socket": {},
	}

	directory, err := data.Assets.Open(uri)
	if err != nil {
		return err
	}
	defer directory.Close()

	children, err := directory.Readdir(0)
	if err != nil {
		return err
	}

	for _, childInfo := range children {
		dir := path.Join(uri, childInfo.Name())
		file, err := data.Assets.Open(dir)
		if err != nil {
			return err
		}
		defer file.Close()

		info, err := file.Stat()
		if err != nil {
			return err
		}

		if info.IsDir() {
			if dir := info.Name(); !strings.HasSuffix(dir, ".d") {
				logrus.Tracef("Ignoring internal asset directory %q while looking for systemd drop-ins", dir)
				continue
			}

			children, err := file.Readdir(0)
			if err != nil {
				return err
			}
			if err = file.Close(); err != nil {
				return err
			}

			dropins := []igntypes.SystemdDropin{}
			for _, childInfo := range children {
				file, err := data.Assets.Open(path.Join(dir, childInfo.Name()))
				if err != nil {
					return err
				}
				defer file.Close()

				childName, contents, err := readFile(childInfo.Name(), file, templateData)
				if err != nil {
					return err
				}

				dropins = append(dropins, igntypes.SystemdDropin{
					Name:     childName,
					Contents: string(contents),
				})
			}

			name := strings.TrimSuffix(childInfo.Name(), ".d")
			unit := igntypes.Unit{
				Name:    name,
				Dropins: dropins,
			}
			if _, ok := enabled[name]; ok {
				unit.Enabled = util.BoolToPtr(true)
			}
			a.Config.Systemd.Units = append(a.Config.Systemd.Units, unit)
		} else {
			name, contents, err := readFile(childInfo.Name(), file, templateData)
			if err != nil {
				return err
			}

			unit := igntypes.Unit{
				Name:     name,
				Contents: string(contents),
			}
			if _, ok := enabled[name]; ok {
				unit.Enabled = util.BoolToPtr(true)
			}
			a.Config.Systemd.Units = append(a.Config.Systemd.Units, unit)
		}
	}

	return nil
}

// Read data from the string reader, and, if the name ends with
// '.template', strip that extension from the name and render the
// template.
func readFile(name string, reader io.Reader, templateData interface{}) (finalName string, data []byte, err error) {
	data, err = ioutil.ReadAll(reader)
	if err != nil {
		return name, []byte{}, err
	}

	if filepath.Ext(name) == ".template" {
		name = strings.TrimSuffix(name, ".template")
		tmpl := template.New(name)
		tmpl, err := tmpl.Parse(string(data))
		if err != nil {
			return name, data, err
		}
		stringData := applyTemplateData(tmpl, templateData)
		data = []byte(stringData)
	}

	return name, data, nil
}

func (a *Bootstrap) addParentFiles(dependencies asset.Parents) {
	for _, asset := range []asset.WritableAsset{
		&manifests.Manifests{},
		&manifests.Openshift{},
		&machines.Master{},
	} {
		dependencies.Get(asset)
		a.Config.Storage.Files = append(a.Config.Storage.Files, ignition.FilesFromAsset(rootDir, "root", 0644, asset)...)
	}

	for _, asset := range []asset.WritableAsset{
		&kubeconfig.AdminClient{},
		&kubeconfig.Kubelet{},
		&tls.AdminKubeConfigCABundle{},
		&tls.AggregatorCA{},
		&tls.AggregatorCABundle{},
		&tls.AggregatorClientCertKey{},
		&tls.AggregatorSignerCertKey{},
		&tls.APIServerCertKey{},
		&tls.APIServerProxyCertKey{},
		&tls.EtcdCA{},
		&tls.EtcdCABundle{},
		&tls.EtcdClientCertKey{},
		&tls.EtcdMetricsCABundle{},
		&tls.EtcdMetricsSignerClientCertKey{},
		&tls.EtcdMetricsSignerServerCertKey{},
		&tls.EtcdSignerCertKey{},
		&tls.EtcdSignerClientCertKey{},
		&tls.KubeAPIServerLBCABundle{},
		&tls.KubeAPIServerLBServerCertKey{},
		&tls.KubeAPIServerLBSignerCertKey{},
		&tls.KubeAPIServerLocalhostCABundle{},
		&tls.KubeAPIServerLocalhostServerCertKey{},
		&tls.KubeAPIServerLocalhostSignerCertKey{},
		&tls.KubeAPIServerServiceNetworkCABundle{},
		&tls.KubeAPIServerServiceNetworkServerCertKey{},
		&tls.KubeAPIServerServiceNetworkSignerCertKey{},
		&tls.KubeAPIServerCompleteCABundle{},
		&tls.KubeAPIServerCompleteClientCABundle{},
		&tls.KubeAPIServerToKubeletCABundle{},
		&tls.KubeAPIServerToKubeletClientCertKey{},
		&tls.KubeAPIServerToKubeletSignerCertKey{},
		&tls.KubeCA{},
		&tls.KubeControlPlaneCABundle{},
		&tls.KubeControlPlaneKubeControllerManagerClientCertKey{},
		&tls.KubeControlPlaneKubeSchedulerClientCertKey{},
		&tls.KubeControlPlaneSignerCertKey{},
		&tls.KubeletBootstrapCABundle{},
		&tls.KubeletClientCABundle{},
		&tls.KubeletClientCertKey{},
		&tls.KubeletCSRSignerCertKey{},
		&tls.KubeletServingCABundle{},
		&tls.MCSCertKey{},
		&tls.ServiceAccountKeyPair{},
	} {
		dependencies.Get(asset)
		a.Config.Storage.Files = append(a.Config.Storage.Files, ignition.FilesFromAsset(rootDir, "root", 0600, asset)...)
	}

	rootCA := &tls.RootCA{}
	dependencies.Get(rootCA)
	a.Config.Storage.Files = append(a.Config.Storage.Files, ignition.FileFromBytes(filepath.Join(rootDir, rootCA.CertFile().Filename), "root", 0644, rootCA.Cert()))

	journal := &tls.JournalCertKey{}
	dependencies.Get(journal)
	a.Config.Storage.Files = append(a.Config.Storage.Files, ignition.FilesFromAsset(rootDir, "systemd-journal-gateway", 0600, journal)...)
}

func applyTemplateData(template *template.Template, templateData interface{}) string {
	buf := &bytes.Buffer{}
	if err := template.Execute(buf, templateData); err != nil {
		panic(err)
	}
	return buf.String()
}

// Load returns the bootstrap ignition from disk.
func (a *Bootstrap) Load(f asset.FileFetcher) (found bool, err error) {
	file, err := f.FetchByName(bootstrapIgnFilename)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	config := &igntypes.Config{}
	if err := json.Unmarshal(file.Data, config); err != nil {
		return false, errors.Wrap(err, "failed to unmarshal")
	}

	a.File, a.Config = file, config
	return true, nil
}
