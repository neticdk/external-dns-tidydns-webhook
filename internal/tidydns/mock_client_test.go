package tidydns

import (
	"context"

	"github.com/neticdk/tidydns-go/pkg/tidydns"
	"github.com/stretchr/testify/mock"
)

type mockClient struct {
	mock.Mock
}

func (m *mockClient) ListZones(ctx context.Context) ([]*tidydns.ZoneInfo, error) {
	args := m.Called(ctx)
	return args.Get(0).([]*tidydns.ZoneInfo), args.Error(1)
}

func (m *mockClient) FindZoneID(ctx context.Context, name string) (int, error) {
	args := m.Called(ctx, name)
	return args.Int(0), args.Error(1)
}

func (m *mockClient) ListRecords(ctx context.Context, zoneID int) ([]*tidydns.RecordInfo, error) {
	args := m.Called(ctx, zoneID)
	return args.Get(0).([]*tidydns.RecordInfo), args.Error(1)
}

func (m *mockClient) CreateRecord(ctx context.Context, zoneID int, info tidydns.RecordInfo) (int, error) {
	args := m.Called(ctx, zoneID, info)
	return args.Int(0), args.Error(1)
}

func (m *mockClient) UpdateRecord(ctx context.Context, zoneID int, recordID int, info tidydns.RecordInfo) error {
	args := m.Called(ctx, zoneID, recordID, info)
	return args.Error(0)
}

func (m *mockClient) ReadRecord(ctx context.Context, zoneID int, recordID int) (*tidydns.RecordInfo, error) {
	args := m.Called(ctx, zoneID, recordID)
	if args.Get(0) != nil {
		return args.Get(0).(*tidydns.RecordInfo), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockClient) FindRecord(ctx context.Context, zoneID int, name string, rType tidydns.RecordType) ([]*tidydns.RecordInfo, error) {
	args := m.Called(ctx, zoneID, name, rType)
	if args.Get(0) != nil {
		return args.Get(0).([]*tidydns.RecordInfo), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockClient) DeleteRecord(ctx context.Context, zoneID int, recordID int) error {
	args := m.Called(ctx, zoneID, recordID)
	return args.Error(0)
}

func (m *mockClient) GetSubnetIDs(ctx context.Context, subnetCIDR string) (*tidydns.SubnetIDs, error) {
	args := m.Called(ctx, subnetCIDR)
	return args.Get(0).(*tidydns.SubnetIDs), args.Error(1)
}

func (m *mockClient) GetFreeIP(ctx context.Context, subnetID int) (string, error) {
	args := m.Called(ctx, subnetID)
	return args.String(0), args.Error(1)
}

func (m *mockClient) ListDHCPInterfaces(ctx context.Context, subnetID int) ([]*tidydns.InterfaceInfo, error) {
	args := m.Called(ctx, subnetID)
	return args.Get(0).([]*tidydns.InterfaceInfo), args.Error(1)
}

func (m *mockClient) CreateDHCPInterface(ctx context.Context, createInfo tidydns.CreateInfo) (int, error) {
	args := m.Called(ctx, createInfo)
	return args.Int(0), args.Error(1)
}

func (m *mockClient) ReadDHCPInterface(ctx context.Context, interfaceID int) (*tidydns.InterfaceInfo, error) {
	args := m.Called(ctx, interfaceID)
	return args.Get(0).(*tidydns.InterfaceInfo), args.Error(1)
}

func (m *mockClient) UpdateDHCPInterfaceName(ctx context.Context, interfaceID int, interfaceName string) (int, error) {
	args := m.Called(ctx, interfaceID, interfaceName)
	return args.Int(0), args.Error(1)
}

func (m *mockClient) DeleteDHCPInterface(ctx context.Context, interfaceID int) error {
	args := m.Called(ctx, interfaceID)
	return args.Error(0)
}

func (m *mockClient) CreateInternalUser(ctx context.Context, username string, password string, description string, changePasswordOnFirstLogin bool, authGroup tidydns.AuthGroup, userAllow []tidydns.UserAllowID) (tidydns.UserID, error) {
	args := m.Called(ctx, username, password, description, changePasswordOnFirstLogin, authGroup, userAllow)
	return args.Get(0).(tidydns.UserID), args.Error(1)
}

func (m *mockClient) GetInternalUser(ctx context.Context, userID tidydns.UserID) (*tidydns.UserInfo, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(*tidydns.UserInfo), args.Error(1)
}

func (m *mockClient) UpdateInternalUser(ctx context.Context, userID tidydns.UserID, password *string, description *string, authGroup *tidydns.AuthGroup, userAllow []tidydns.UserAllowID) error {
	args := m.Called(ctx, userID, password, description, authGroup, userAllow)
	return args.Error(0)
}

func (m *mockClient) DeleteInternalUser(ctx context.Context, userID tidydns.UserID) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}
