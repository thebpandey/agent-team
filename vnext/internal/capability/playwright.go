package capability

import "fmt"

type BrowserResource struct {
	ID, Session, State, Ownership string
}

type PlaywrightQuestion struct {
	Question
	SessionName, ResourceID, TargetURL, Viewport string
}

type PlaywrightRoute struct {
	SessionName, ResourceID, TargetURL, Viewport string
	EvidencePointer                              string
}

func RoutePlaywright(q PlaywrightQuestion, resources []BrowserResource) (PlaywrightRoute, error) {
	if q.Task == "" || q.SessionName == "" || q.ResourceID == "" || q.TargetURL == "" || q.Viewport == "" {
		return PlaywrightRoute{}, fmt.Errorf("playwright route requires named managed evidence")
	}
	for _, resource := range resources {
		if resource.ID == q.ResourceID && resource.Session == q.SessionName && resource.State == "ready" && resource.Ownership == "managed" {
			return PlaywrightRoute{SessionName: q.SessionName, ResourceID: q.ResourceID, TargetURL: q.TargetURL, Viewport: q.Viewport, EvidencePointer: "browser:" + q.ResourceID}, nil
		}
	}
	return PlaywrightRoute{}, fmt.Errorf("playwright resource is not owned and ready")
}
