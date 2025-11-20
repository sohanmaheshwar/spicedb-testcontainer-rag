package rag

import (
	"context"
	"strings"

	apiv1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	authzed "github.com/authzed/authzed-go/v1"
)

// Document is a trivial "chunk" for the RAG pipeline.
type Document struct {
	ID       string
	Text     string
	Metadata map[string]string
}

// RAGPipeline holds docs and a SpiceDB client used for access checks.
type RAGPipeline struct {
	docs         []Document
	spiceClient  *authzed.Client
	resourceType string // e.g. "document"
	permission   string // e.g. "read"
}

// NewRAGPipeline constructs a new pipeline.
func NewRAGPipeline(spiceClient *authzed.Client, resourceType, permission string, docs []Document) *RAGPipeline {
	return &RAGPipeline{
		docs:         docs,
		spiceClient:  spiceClient,
		resourceType: resourceType,
		permission:   permission,
	}
}

// Query performs a trivial "retrieval" and then filters with SpiceDB.
// - retrieval: substring match on Text
// - filtering: CheckPermission(user, permission, resource) via SpiceDB
func (r *RAGPipeline) Query(ctx context.Context, userID, query string) ([]Document, error) {
	var candidates []Document
	lq := strings.ToLower(query)

	// naive retrieval
	for _, d := range r.docs {
		if strings.Contains(strings.ToLower(d.Text), lq) {
			candidates = append(candidates, d)
		}
	}

	var allowed []Document

	for _, d := range candidates {
		spiceObj := d.Metadata["spicedb_object"]
		if spiceObj == "" {
			// If there's no SpiceDB mapping, treat as non-readable
			continue
		}

		// We store IDs as e.g. "document:doc1"
		parts := strings.SplitN(spiceObj, ":", 2)
		if len(parts) != 2 {
			continue
		}
		objType, objID := parts[0], parts[1]

		res := &apiv1.ObjectReference{
			ObjectType: objType,
			ObjectId:   objID,
		}
		subject := &apiv1.SubjectReference{
			Object: &apiv1.ObjectReference{
				ObjectType: "user",
				ObjectId:   userID,
			},
		}

		resp, err := r.spiceClient.CheckPermission(ctx, &apiv1.CheckPermissionRequest{
			Resource:   res,
			Permission: r.permission,
			Subject:    subject,
		})
		if err != nil {
			return nil, err
		}

		if resp.Permissionship == apiv1.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION {
			allowed = append(allowed, d)
		}
	}

	return allowed, nil
}
