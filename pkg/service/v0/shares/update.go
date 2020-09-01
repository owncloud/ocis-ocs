package shares

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	revarpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	revacollaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	revalink "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	revaprovider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	revatypes "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/go-chi/chi"
	"github.com/go-chi/render"
	"github.com/owncloud/ocis-ocs/pkg/service/v0/data"
	"github.com/owncloud/ocis-ocs/pkg/service/v0/response"
)

func (s *Service) updateShare(w http.ResponseWriter, r *http.Request) {
	shareID := chi.URLParam(r, "shareID")

	err := r.ParseForm()
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaBadRequest.StatusCode,
			fmt.Sprintf("could not parse form from request: %s", err),
		))
		return
	}

	psRes, err := s.client.GetPublicShare(r.Context(), &revalink.GetPublicShareRequest{
		Ref: &revalink.PublicShareReference{
			Spec: &revalink.PublicShareReference_Id{
				Id: &revalink.PublicShareId{
					OpaqueId: shareID,
				},
			},
		},
	})
	if err != nil {
		s.logger.Err(err).Str("shareID", shareID).Msg("failed to get public share")
	}

	if psRes.GetShare() != nil {
		s.updatePublicShare(w, r, psRes.GetShare())
		return
	}

	uRes, err := s.client.GetShare(r.Context(), &revacollaboration.GetShareRequest{
		Ref: &revacollaboration.ShareReference{
			Spec: &revacollaboration.ShareReference_Id{
				Id: &revacollaboration.ShareId{
					OpaqueId: shareID,
				},
			},
		},
	})
	if err != nil {
		s.logger.Err(err).Str("shareID", shareID).Msg("failed to get share")
	}

	if uRes.GetShare() != nil {
		s.updateUserShare(w, r, uRes.GetShare())
		return
	}

	render.Render(w, r, response.ErrRender(data.MetaNotFound.StatusCode, "could not find share"))
}

func (s *Service) updatePublicShare(w http.ResponseWriter, r *http.Request, share *revalink.PublicShare) {
	updates := []*revalink.UpdatePublicShareRequest_Update{}
	// indicates whether values to update were found,
	// to check if the request was valid,
	// not whether an actual update has been performed
	updatesFound := false

	newName, ok := r.Form["name"]
	if ok {
		updatesFound = true
		if newName[0] != share.DisplayName {
			updates = append(updates, &revalink.UpdatePublicShareRequest_Update{
				Type:        revalink.UpdatePublicShareRequest_Update_TYPE_DISPLAYNAME,
				DisplayName: newName[0],
			})
		}
	}

	newPermissions, err := permissionsFromRequest(r)
	s.logger.Debug().Interface("newPermissions", newPermissions).Msg("Parsed permissions")
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaBadRequest.StatusCode,
			fmt.Sprintf("invalid permissions: %s", err),
		))
		return
	}

	// update permissions if given
	if newPermissions != nil {
		updatesFound = true
		publicSharePermissions := &revalink.PublicSharePermissions{
			Permissions: newPermissions,
		}
		beforePerm, _ := json.Marshal(share.Permissions)
		afterPerm, _ := json.Marshal(publicSharePermissions)
		if string(beforePerm) != string(afterPerm) {
			s.logger.Info().Str("shares", "update").Msgf("updating permissions from %v to: %v", string(beforePerm), string(afterPerm))
			updates = append(updates, &revalink.UpdatePublicShareRequest_Update{
				Type: revalink.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS,
				Grant: &revalink.Grant{
					Permissions: publicSharePermissions,
				},
			})
		}
	}

	// ExpireDate
	expireTimeString, ok := r.Form["expireDate"]
	// check if value is set and must be updated or cleared
	if ok {
		updatesFound = true
		var newExpiration *revatypes.Timestamp
		if expireTimeString[0] != "" {
			newExpiration, err = parseTimestamp(expireTimeString[0])
			if err != nil {
				render.Render(w, r, response.ErrRender(
					data.MetaBadRequest.StatusCode,
					fmt.Sprintf("invalid datetime format: %s", err),
				))
				return
			}
		}

		beforeExpiration, _ := json.Marshal(share.Expiration)
		afterExpiration, _ := json.Marshal(newExpiration)
		if string(afterExpiration) != string(beforeExpiration) {
			s.logger.
				Debug().
				Str("shares", "update").
				Msgf("updating expire date from %v to: %v", string(beforeExpiration), string(afterExpiration))
			updates = append(updates, &revalink.UpdatePublicShareRequest_Update{
				Type: revalink.UpdatePublicShareRequest_Update_TYPE_EXPIRATION,
				Grant: &revalink.Grant{
					Expiration: newExpiration,
				},
			})
		}
	}

	// Password
	newPassword, ok := r.Form["password"]
	// update or clear password
	if ok {
		updatesFound = true
		s.logger.Info().Str("shares", "update").Msg("password updated")
		updates = append(updates, &revalink.UpdatePublicShareRequest_Update{
			Type: revalink.UpdatePublicShareRequest_Update_TYPE_PASSWORD,
			Grant: &revalink.Grant{
				Password: newPassword[0],
			},
		})
	}

	updatedShare := share
	// Updates are atomical. See https://github.com/cs3org/cs3apis/pull/67#issuecomment-617651428 so in order to get the latest updated version
	if len(updates) > 0 {
		uRes := &revalink.UpdatePublicShareResponse{Share: share}
		for k := range updates {
			uRes, err = s.client.UpdatePublicShare(r.Context(), &revalink.UpdatePublicShareRequest{
				Ref: &revalink.PublicShareReference{
					Spec: &revalink.PublicShareReference_Id{
						Id: &revalink.PublicShareId{
							OpaqueId: share.Id.OpaqueId,
						},
					},
				},
				Update: updates[k],
			})
			if err != nil {
				s.logger.Err(err).Str("shareID", share.Id.OpaqueId).Msg("sending update request to public link provider")
				render.Render(w, r, response.ErrRender(
					data.MetaServerError.StatusCode,
					fmt.Sprintf("error sending update request to public link provider: %s", err),
				))
				return
			}
		}
		updatedShare = uRes.Share
	} else if !updatesFound {
		render.Render(w, r, response.ErrRender(data.MetaBadRequest.StatusCode, "No updates specified in request"))
		return
	}

	statReq := revaprovider.StatRequest{
		Ref: &revaprovider.Reference{
			Spec: &revaprovider.Reference_Id{
				Id: share.ResourceId,
			},
		},
	}

	statRes, err := s.client.Stat(r.Context(), &statReq)
	if err != nil {
		s.logger.Debug().Err(err).Str("shares", "update public share").Msg("error during stat")
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("missing resource information: %s", err),
		))
		return
	}

	sd := publicShare2ShareData(updatedShare, s.publicURL)
	if err = addFileInfo(r.Context(), s.client, sd, statRes.Info); err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("error enhancing response with share data: %s", err),
		))
		return
	}

	render.Render(w, r, response.DataRender(sd))
}

func (s *Service) updateUserShare(w http.ResponseWriter, r *http.Request, share *revacollaboration.Share) {
	pval := r.FormValue("permissions")
	if pval == "" {
		render.Render(w, r, response.ErrRender(data.MetaBadRequest.StatusCode, "permissions missing"))
		return
	}

	pint, err := strconv.Atoi(pval)
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaBadRequest.StatusCode,
			fmt.Sprintf("permissions must be an integer: %s", err),
		))
		return
	}
	permissions, err := data.NewPermissions(pint)
	if err != nil {
		render.Render(w, r, response.ErrRender(data.MetaBadRequest.StatusCode, err.Error()))
		return
	}

	uReq := &revacollaboration.UpdateShareRequest{
		Ref: &revacollaboration.ShareReference{
			Spec: &revacollaboration.ShareReference_Id{
				Id: &revacollaboration.ShareId{
					OpaqueId: share.Id.OpaqueId,
				},
			},
		},
		Field: &revacollaboration.UpdateShareRequest_UpdateField{
			Field: &revacollaboration.UpdateShareRequest_UpdateField_Permissions{
				// this completely overwrites the permissions for this user
				Permissions: &revacollaboration.SharePermissions{
					Permissions: asCS3Permissions(permissions, nil),
				},
			},
		},
	}
	uRes, err := s.client.UpdateShare(r.Context(), uReq)
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("error sending a grpc update share request: %s", err),
		))
		return
	}

	if uRes.Status.Code != revarpc.Code_CODE_OK {
		if uRes.Status.Code == revarpc.Code_CODE_NOT_FOUND {
			render.Render(w, r, response.ErrRender(data.MetaNotFound.StatusCode, "not found"))
			return
		}
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("grpc update share request failed: %s", err),
		))
		return
	}

	gReq := &revacollaboration.GetShareRequest{
		Ref: &revacollaboration.ShareReference{
			Spec: &revacollaboration.ShareReference_Id{
				Id: &revacollaboration.ShareId{
					OpaqueId: share.Id.OpaqueId,
				},
			},
		},
	}
	gRes, err := s.client.GetShare(r.Context(), gReq)
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("error sending a grpc get share request: %s", err),
		))
		return
	}

	if gRes.Status.Code != revarpc.Code_CODE_OK {
		if gRes.Status.Code == revarpc.Code_CODE_NOT_FOUND {
			render.Render(w, r, response.ErrRender(data.MetaNotFound.StatusCode, "not found"))
			return
		}
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("grpc get share request failed: %s", err),
		))
		return
	}

	sd, err := userShare2ShareData(r.Context(), s.client, gRes.Share)
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("error mapping share data: %s", err),
		))
		return
	}

	statReq := revaprovider.StatRequest{
		Ref: &revaprovider.Reference{
			Spec: &revaprovider.Reference_Id{
				Id: gRes.GetShare().ResourceId,
			},
		},
	}

	statRes, err := s.client.Stat(r.Context(), &statReq)
	if err != nil {
		s.logger.Debug().Err(err).Str("shares", "update user share").Msg("error during stat")
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("error getting resource information: %s", err),
		))
		return
	}

	if statRes.Status.Code != revarpc.Code_CODE_OK {
		if statRes.Status.Code == revarpc.Code_CODE_NOT_FOUND {
			render.Render(w, r, response.ErrRender(
				data.MetaNotFound.StatusCode,
				"update user share: resource not found",
			))
			return
		}
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("grpc stat request failed for stat after updating user share: %s", err),
		))
		return
	}

	err = addFileInfo(r.Context(), s.client, sd, statRes.Info)
	if err != nil {
		render.Render(w, r, response.ErrRender(data.MetaServerError.StatusCode, err.Error()))
		return
	}
	render.Render(w, r, response.DataRender(sd))
}
