package shares

import (
	"fmt"
	"net/http"

	revarpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	revacollaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	revalink "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	"github.com/go-chi/chi"
	"github.com/go-chi/render"
	"github.com/owncloud/ocis-ocs/pkg/service/v0/data"
	"github.com/owncloud/ocis-ocs/pkg/service/v0/response"
)

func (s *Service) removeShare(w http.ResponseWriter, r *http.Request) {
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
		s.removePublicShare(w, r, psRes.GetShare())
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
		s.removeUserShare(w, r, uRes.GetShare())
	}
	render.Render(w, r, response.ErrRender(data.MetaNotFound.StatusCode, "could not find share"))
}

func (s *Service) removePublicShare(w http.ResponseWriter, r *http.Request, share *revalink.PublicShare) {
	req := &revalink.RemovePublicShareRequest{
		Ref: &revalink.PublicShareReference{
			Spec: &revalink.PublicShareReference_Id{
				Id: &revalink.PublicShareId{
					OpaqueId: share.Id.OpaqueId,
				},
			},
		},
	}

	res, err := s.client.RemovePublicShare(r.Context(), req)
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("error sending a grpc delete share request: %s", err),
		))
		return
	}
	if res.GetStatus().GetCode() != revarpc.Code_CODE_OK {
		if res.GetStatus().GetCode() == revarpc.Code_CODE_NOT_FOUND {
			render.Render(w, r, response.ErrRender(data.MetaNotFound.StatusCode, "not found"))
			return
		}
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("grpc delete share request failed: %s", res.GetStatus().GetMessage()),
		))
		return
	}

	render.Render(w, r, response.DataRender(nil))
}

func (s *Service) removeUserShare(w http.ResponseWriter, r *http.Request, share *revacollaboration.Share) {
	uReq := &revacollaboration.RemoveShareRequest{
		Ref: &revacollaboration.ShareReference{
			Spec: &revacollaboration.ShareReference_Id{
				Id: &revacollaboration.ShareId{
					OpaqueId: share.Id.OpaqueId,
				},
			},
		},
	}

	uRes, err := s.client.RemoveShare(r.Context(), uReq)
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("error sending a grpc delete share request: %s", err),
		))
		return
	}

	if uRes.GetStatus().GetCode() != revarpc.Code_CODE_OK {
		if uRes.GetStatus().GetCode() == revarpc.Code_CODE_NOT_FOUND {
			render.Render(w, r, response.ErrRender(data.MetaNotFound.StatusCode, "not found"))
			return
		}
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("grpc delete share request failed: %s", uRes.GetStatus().GetMessage()),
		))
		return
	}
	render.Render(w, r, response.DataRender(nil))
}
