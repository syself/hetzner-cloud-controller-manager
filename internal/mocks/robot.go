package mocks

import (
	"github.com/stretchr/testify/mock"
	"github.com/syself/hrobot-go/models"
)

type RobotClient struct {
	mock.Mock
}

func (m *RobotClient) ServerGetList() ([]models.Server, error) {
	args := m.Called()
	return getRobotServers(args, 0), args.Error(1)
}

func (m *RobotClient) SetCredentials(_, _ string) error {
	args := m.Called()
	return args.Error(3)
}

func (m *RobotClient) BootLinuxDelete(_ int) (*models.Linux, error) {
	panic("this method should not be called")
}

func (m *RobotClient) BootLinuxGet(_ int) (*models.Linux, error) {
	panic("this method should not be called")
}

func (m *RobotClient) BootLinuxSet(_ int, _ *models.LinuxSetInput) (*models.Linux, error) {
	panic("this method should not be called")
}

func (m *RobotClient) BootRescueDelete(_ int) (*models.Rescue, error) {
	panic("this method should not be called")
}

func (m *RobotClient) BootRescueGet(_ int) (*models.Rescue, error) {
	panic("this method should not be called")
}

func (m *RobotClient) BootRescueSet(_ int, _ *models.RescueSetInput) (*models.Rescue, error) {
	panic("this method should not be called")
}

func (m *RobotClient) FailoverGet(_ string) (*models.Failover, error) {
	panic("this method should not be called")
}

func (m *RobotClient) FailoverGetList() ([]models.Failover, error) {
	panic("this method should not be called")
}

func (m *RobotClient) GetVersion() string {
	panic("this method should not be called")
}

func (m *RobotClient) IPGetList() ([]models.IP, error) {
	panic("this method should not be called")
}

func (m *RobotClient) KeyGetList() ([]models.Key, error) {
	panic("this method should not be called")
}

func (m *RobotClient) KeySet(_ *models.KeySetInput) (*models.Key, error) {
	panic("this method should not be called")
}

func (m *RobotClient) RDnsGet(_ string) (*models.Rdns, error) {
	panic("this method should not be called")
}

func (m *RobotClient) RDnsGetList() ([]models.Rdns, error) {
	panic("this method should not be called")
}

func (m *RobotClient) ResetGet(_ int) (*models.Reset, error) {
	panic("this method should not be called")
}

func (m *RobotClient) ResetSet(_ int, _ *models.ResetSetInput) (*models.ResetPost, error) {
	panic("this method should not be called")
}

func (m *RobotClient) ServerGet(_ int) (*models.Server, error) {
	panic("this method should not be called")
}

func (m *RobotClient) ServerReverse(_ int) (*models.Cancellation, error) {
	panic("this method should not be called")
}

func (m *RobotClient) ServerSetName(_ int, _ *models.ServerSetNameInput) (*models.Server, error) {
	panic("this method should not be called")
}

func (m *RobotClient) SetBaseURL(_ string) {
	panic("this method should not be called")
}

func (m *RobotClient) SetUserAgent(_ string) {
	panic("this method should not be called")
}

func (m *RobotClient) ValidateCredentials() error {
	panic("this method should not be called")
}
