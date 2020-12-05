package data

import "fmt"

// Permissions reflects the CRUD permissions used in the OCS sharing API
type Permissions uint

const (
	// PermissionInvalid grants no permissions on a resource
	PermissionInvalid Permissions = 0
	// PermissionRead grants read permissions on a resource
	PermissionRead Permissions = 1 << (iota - 1)
	// PermissionWrite grants write permissions on a resource
	PermissionWrite
	// PermissionCreate grants create permissions on a resource
	PermissionCreate
	// PermissionDelete grants delete permissions on a resource
	PermissionDelete
	// PermissionShare grants share permissions on a resource
	PermissionShare
	// PermissionAll grants all permissions on a resource
	PermissionAll Permissions = (1 << (iota - 1)) - 1

	// ShareTypeUser refers to user shares
	ShareTypeUser ShareType = 0

	// ShareTypePublicLink refers to public link shares
	ShareTypePublicLink ShareType = 3

	// ShareTypeGroup represents a group share
	// ShareTypeGroup ShareType = 1

	// ShareTypeFederatedCloudShare represents a federated share
	ShareTypeFederatedCloudShare ShareType = 6

	// RoleLegacy provides backwards compatibility
	RoleLegacy string = "legacy"
	// RoleViewer grants non-editor role on a resource
	RoleViewer string = "viewer"
	// RoleEditor grants editor permission on a resource
	RoleEditor string = "editor"
	// RoleCoowner grants owner permissions on a resource
	RoleCoowner string = "coowner"
)

var (
	// ErrPermissionNotInRange defines a permission specific error.
	ErrPermissionNotInRange = fmt.Errorf("the provided permission is not between %d and %d", PermissionInvalid, PermissionAll)
)

// ShareType denotes a type of share
type ShareType int

// ShareData represents https://doc.owncloud.com/server/developer_manual/core/ocs-share-api.html#response-attributes-1
type ShareData struct {
	// TODO int?
	ID string `json:"id" xml:"id"`
	// The share’s type
	ShareType ShareType `json:"share_type" xml:"share_type"`
	// The username of the owner of the share.
	UIDOwner string `json:"uid_owner" xml:"uid_owner"`
	// The display name of the owner of the share.
	DisplaynameOwner string `json:"displayname_owner" xml:"displayname_owner"`
	// The permission attribute set on the file.
	// TODO(jfd) change the default to read only
	Permissions Permissions `json:"permissions" xml:"permissions"`
	// The UNIX timestamp when the share was created.
	STime uint64 `json:"stime" xml:"stime"`
	// ?
	Parent string `json:"parent" xml:"parent"`
	// The UNIX timestamp when the share expires.
	Expiration string `json:"expiration" xml:"expiration"`
	// The public link to the item being shared.
	Token string `json:"token" xml:"token"`
	// The unique id of the user that owns the file or folder being shared.
	UIDFileOwner string `json:"uid_file_owner" xml:"uid_file_owner"`
	// The display name of the user that owns the file or folder being shared.
	DisplaynameFileOwner string `json:"displayname_file_owner" xml:"displayname_file_owner"`
	// ?
	AdditionalInfoOwner string `json:"additional_info_owner" xml:"additional_info_owner"`
	// ?
	AdditionalInfoFileOwner string `json:"additional_info_file_owner" xml:"additional_info_file_owner"`
	// share state, 0 = accepted, 1 = pending, 2 = declined
	State int `json:"state" xml:"state"`
	// The path to the shared file or folder.
	Path string `json:"path" xml:"path"`
	// The type of the object being shared. This can be one of 'file' or 'folder'.
	ItemType string `json:"item_type" xml:"item_type"`
	// The RFC2045-compliant mimetype of the file.
	MimeType  string `json:"mimetype" xml:"mimetype"`
	StorageID string `json:"storage_id" xml:"storage_id"`
	Storage   uint64 `json:"storage" xml:"storage"`
	// The unique node id of the item being shared.
	ItemSource string `json:"item_source" xml:"item_source"`
	// The unique node id of the item being shared. For legacy reasons item_source and file_source attributes have the same value.
	FileSource string `json:"file_source" xml:"file_source"`
	// The unique node id of the parent node of the item being shared.
	FileParent string `json:"file_parent" xml:"file_parent"`
	// The basename of the shared file.
	FileTarget string `json:"file_target" xml:"file_target"`
	// The uid of the receiver of the file. This is either
	// - a GID (group id) if it is being shared with a group or
	// - a UID (user id) if the share is shared with a user.
	ShareWith string `json:"share_with,omitempty" xml:"share_with,omitempty"`
	// The display name of the receiver of the file.
	ShareWithDisplayname string `json:"share_with_displayname,omitempty" xml:"share_with_displayname,omitempty"`
	// sharee Additional info
	ShareWithAdditionalInfo string `json:"share_with_additional_info" xml:"share_with_additional_info"`
	// Whether the recipient was notified, by mail, about the share being shared with them.
	MailSend int `json:"mail_send" xml:"mail_send"`
	// Name of the public share
	Name string `json:"name" xml:"name"`
	// URL of the public share
	URL string `json:"url,omitempty" xml:"url,omitempty"`
	// Attributes associated
	Attributes string `json:"attributes,omitempty" xml:"attributes,omitempty"`
	// PasswordProtected represents a public share is password protected
	// PasswordProtected bool `json:"password_protected,omitempty" xml:"password_protected,omitempty"`
}

// ResourceType indicates the OCS type of the resource
type ResourceType int

func (rt ResourceType) String() (s string) {
	switch rt {
	case 0:
		s = "invalid"
	case 1:
		s = "file"
	case 2:
		s = "folder"
	case 3:
		s = "reference"
	default:
		s = "invalid"
	}
	return
}

// NewPermissions creates a new Permissions instance.
// The value must be in the valid range.
func NewPermissions(val int) (Permissions, error) {
	if val == int(PermissionInvalid) {
		return PermissionInvalid, fmt.Errorf("permissions %d out of range %d - %d", val, PermissionRead, PermissionAll)
	} else if val < int(PermissionInvalid) || int(PermissionAll) < val {
		return PermissionInvalid, ErrPermissionNotInRange
	}
	return Permissions(val), nil
}

// Contain tests if the permissions contain another one.
func (p Permissions) Contain(other Permissions) bool {
	return p&other != 0
}

// Permissions2Role performs permission conversions for user and federated shares
func Permissions2Role(p Permissions) string {
	role := RoleLegacy
	if p.Contain(PermissionRead) {
		role = RoleViewer
	}
	if p.Contain(PermissionWrite) {
		role = RoleEditor
	}
	if p.Contain(PermissionShare) {
		role = RoleCoowner
	}
	return role
}
