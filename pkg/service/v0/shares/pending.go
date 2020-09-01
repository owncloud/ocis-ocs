package shares

import (
	"fmt"
	"net/http"

	revarpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	revacollaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	"github.com/go-chi/chi"
	"github.com/go-chi/render"
	"github.com/owncloud/ocis-ocs/pkg/service/v0/data"
	"github.com/owncloud/ocis-ocs/pkg/service/v0/response"
)

func (s *Service) acceptShare(w http.ResponseWriter, r *http.Request) {
	shareID := chi.URLParam(r, "shareID")
	s.logger.Debug().Str("share_id", shareID).Str("url_path", r.URL.Path).Msg("http routing")

	ctx := r.Context()

	shareRequest := &revacollaboration.UpdateReceivedShareRequest{
		Ref: &revacollaboration.ShareReference{
			Spec: &revacollaboration.ShareReference_Id{
				Id: &revacollaboration.ShareId{
					OpaqueId: shareID,
				},
			},
		},
		Field: &revacollaboration.UpdateReceivedShareRequest_UpdateField{
			Field: &revacollaboration.UpdateReceivedShareRequest_UpdateField_State{
				State: revacollaboration.ShareState_SHARE_STATE_ACCEPTED,
			},
		},
	}

	shareRes, err := s.client.UpdateReceivedShare(ctx, shareRequest)
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("grpc update received share request (accept) failed: %s", err)),
		)
		return
	}

	if shareRes.Status.Code != revarpc.Code_CODE_OK {
		if shareRes.Status.Code == revarpc.Code_CODE_NOT_FOUND {
			render.Render(w, r, response.ErrRender(data.MetaNotFound.StatusCode, "not found"))
			return
		}
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("grpc update received share request (accept) failed: %s", shareRes.Status.Message),
		))
		return
	}
}

func (s *Service) rejectShare(w http.ResponseWriter, r *http.Request) {
	shareID := chi.URLParam(r, "shareID")
	s.logger.Debug().Str("share_id", shareID).Str("url_path", r.URL.Path).Msg("http routing")

	ctx := r.Context()

	shareRequest := &revacollaboration.UpdateReceivedShareRequest{
		Ref: &revacollaboration.ShareReference{
			Spec: &revacollaboration.ShareReference_Id{
				Id: &revacollaboration.ShareId{
					OpaqueId: shareID,
				},
			},
		},
		Field: &revacollaboration.UpdateReceivedShareRequest_UpdateField{
			Field: &revacollaboration.UpdateReceivedShareRequest_UpdateField_State{
				State: revacollaboration.ShareState_SHARE_STATE_REJECTED,
			},
		},
	}

	shareRes, err := s.client.UpdateReceivedShare(ctx, shareRequest)
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("grpc update received share request (reject) failed: %s", err),
		))
		return
	}

	if shareRes.Status.Code != revarpc.Code_CODE_OK {
		if shareRes.Status.Code == revarpc.Code_CODE_NOT_FOUND {
			render.Render(w, r, response.ErrRender(data.MetaNotFound.StatusCode, "not found"))
			return
		}
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("grpc update received share request (reject) failed: %s", shareRes.Status.Message),
		))
		return
	}
}
