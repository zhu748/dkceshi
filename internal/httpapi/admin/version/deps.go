package version

import (
	"context"

	"ds2api/internal/chathistory"
	adminshared "ds2api/internal/httpapi/admin/shared"
)

type Handler struct {
	Store        adminshared.ConfigStore
	Pool         adminshared.PoolController
	DS           adminshared.DeepSeekCaller
	OpenAI       adminshared.OpenAIChatCaller
	ChatHistory  *chathistory.Store
	releases     releaseCache
	checkRelease func(context.Context) (latestReleasePayload, error)
}

var writeJSON = adminshared.WriteJSON
