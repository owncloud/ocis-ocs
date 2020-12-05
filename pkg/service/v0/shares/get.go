package shares

import (
	"fmt"
	"net/http"

	revarpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	revacollaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	revalink "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	revaprovider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/go-chi/chi"
	"github.com/go-chi/render"
	"github.com/owncloud/ocis-ocs/pkg/service/v0/data"
	"github.com/owncloud/ocis-ocs/pkg/service/v0/response"
)

func (s *Service) getShare(w http.ResponseWriter, r *http.Request) {
	shareID := chi.URLParam(r, "shareID")
	ctx := r.Context()

	s.logger.Debug().Str("shareID", shareID).Msg("get public share by id")

	psRes, err := s.client.GetPublicShare(ctx, &revalink.GetPublicShareRequest{
		Ref: &revalink.PublicShareReference{
			Spec: &revalink.PublicShareReference_Id{
				Id: &revalink.PublicShareId{
					OpaqueId: shareID,
				},
			},
		},
	})

	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("error making GetPublicShare grpc request: %s", err),
		))
		return
	}

	if psRes.Status.Code != revarpc.Code_CODE_OK {
		if psRes.Status.Code == revarpc.Code_CODE_NOT_FOUND {
			render.Render(w, r, response.ErrRender(data.MetaNotFound.StatusCode, "not found"))
			return
		}
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("grpc get public share request failed: %s", psRes.Status.Message),
		))
		return
	}

	share := publicShare2ShareData(psRes.Share, s.publicURL)
	resourceID := psRes.Share.ResourceId

	if share == nil {
		s.logger.Debug().Str("shareID", shareID).Msg("get user share by id")
		uRes, err := s.client.GetShare(ctx, &revacollaboration.GetShareRequest{
			Ref: &revacollaboration.ShareReference{
				Spec: &revacollaboration.ShareReference_Id{
					Id: &revacollaboration.ShareId{
						OpaqueId: shareID,
					},
				},
			},
		})

		if err != nil {
			render.Render(w, r, response.ErrRender(
				data.MetaServerError.StatusCode,
				fmt.Sprintf("error making GetShare grpc request: %s", err),
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
				fmt.Sprintf("grpc get user share request failed: %s", uRes.Status.Message),
			))
			return
		}

		resourceID = uRes.Share.ResourceId
		share, err = userShare2ShareData(ctx, s.client, uRes.Share)
		if err != nil {
			render.Render(w, r, response.ErrRender(
				data.MetaServerError.StatusCode,
				fmt.Sprintf("error mapping share data: %s", err),
			))
			return
		}

		if share == nil {
			s.logger.Debug().Str("shareID", shareID).Msg("no share found with this id")
			render.Render(w, r, response.ErrRender(data.MetaNotFound.StatusCode, "share not found"))
			return
		}

		statReq := &revaprovider.StatRequest{
			Ref: &revaprovider.Reference{
				Spec: &revaprovider.Reference_Id{
					Id: resourceID,
				},
			},
		}

		statResponse, err := s.client.Stat(ctx, statReq)
		if err != nil {
			s.logger.Error().Err(err).Msg("error mapping share data")
			render.Render(w, r, response.ErrRender(
				data.MetaServerError.StatusCode,
				fmt.Sprintf("error mapping share data: %s", err),
			))
			return
		}

		if statResponse.Status.Code != revarpc.Code_CODE_OK {
			s.logger.Error().
				Str("error_msg", statResponse.Status.Message).
				Str("status", statResponse.Status.Code.String()).
				Msg("error mapping share data")
			render.Render(w, r, response.ErrRender(
				data.MetaServerError.StatusCode,
				fmt.Sprintf("error mapping share data: %s", statResponse.Status.Message),
			))
			return
		}

		err = addFileInfo(ctx, s.client, share, statResponse.Info)
		if err != nil {
			s.logger.Error().Err(err).Str("status", statResponse.Status.Code.String()).Msg("error mapping share data")
			render.Render(w, r, response.ErrRender(
				data.MetaServerError.StatusCode,
				fmt.Sprintf("error mapping share data: %s", err),
			))
			return
		}

		render.Render(w, r, response.DataRender([]*data.ShareData{share}))
	}

}
