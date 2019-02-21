// +build baremetal

package baremetal

import (
	"github.com/sirupsen/logrus"

	"github.com/openshift-metalkube/kni-installer/pkg/destroy/providers"
	"github.com/openshift-metalkube/kni-installer/pkg/types"
)

// ClusterUninstaller holds the various options for the cluster we want to delete.
type ClusterUninstaller struct {
	Logger logrus.FieldLogger
}

// Run is the entrypoint to start the uninstall process.
func (o *ClusterUninstaller) Run() error {
	o.Logger.Debug("Deleting bare metal resources")
	return nil
}

// New returns bare metal Uninstaller from ClusterMetadata.
func New(logger logrus.FieldLogger, metadata *types.ClusterMetadata) (providers.Destroyer, error) {
	return &ClusterUninstaller{
		Logger: logger,
	}, nil
}