package rag_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	spicedbcontainer "github.com/Mariscal6/testcontainers-spicedb-go"
	apiv1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	authzed "github.com/authzed/authzed-go/v1"
	"github.com/authzed/grpcutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/sohanmaheshwar/rag-spicedb-testcontainers"
)

// Shared test constants
const (
	testImage       = "authzed/spicedb:v1.46.2" // or any recent SpiceDB image
	testPreshared   = "somepresharedkey"
	spiceDBTypeDoc  = "document"
	spiceDBPermRead = "read"
)

// TestRAGWithSpiceDBPermissions demonstrates how the RAG results
// change depending on the calling user, while using a SpiceDB
// Testcontainer to back permission checks.
func TestRAGWithSpiceDBPermissions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// 1. Start SpiceDB via the community Testcontainers module
	container, err := spicedbcontainer.Run(ctx, testImage)
	require.NoError(t, err, "failed to start SpiceDB container")

	{
		logs, err := container.Logs(ctx)
		require.NoError(t, err)

		buf := new(bytes.Buffer)
		_, _ = io.Copy(buf, logs)

		t.Logf("=== SpiceDB Container Logs ===\n%s\n===============================\n", buf.String())
	}

	defer func() { _ = container.Terminate(ctx) }()

	// Discover host:port for gRPC (50051 inside container)
	host, err := container.Host(ctx)
	require.NoError(t, err)

	mappedPort, err := container.MappedPort(ctx, "50051/tcp")
	require.NoError(t, err)

	endpoint := fmt.Sprintf("%s:%s", host, mappedPort.Port())

	// 2. Connect to this SpiceDB using the insecure local pattern
	client, err := authzed.NewClient(
		endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpcutil.WithInsecureBearerToken(testPreshared),
	)
	require.NoError(t, err, "failed to create authzed client")

	// Give SpiceDB a moment if needed (depending on config)
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// 3. Write a minimal schema + relationships:
	//
	// definition user {}
	//
	// definition document {
	//   relation owner: user
	//   relation viewer: user | owner
	//   permission read = owner + viewer
	// }
	//
	// Emilia owns doc1, Beatrice can view doc2, everyone can view doc3.
	writeTestSchema(t, ctx, client)
	writeTestTuples(t, ctx, client)

	// 4. Prepare 3 documents for the RAG index.
	//
	// Important: metadata.spicedb_object matches the SpiceDB object IDs we wrote.
	docs := []rag.Document{
		{
			ID:   "doc1",
			Text: "Internal roadmap for 2025. Highly confidential.",
			Metadata: map[string]string{
				"spicedb_object": "document:doc1",
			},
		},
		{
			ID:   "doc2",
			Text: "Customer success playbook and escalation procedures.",
			Metadata: map[string]string{
				"spicedb_object": "document:doc2",
			},
		},
		{
			ID:   "doc3",
			Text: "Public FAQ for all users.",
			Metadata: map[string]string{
				"spicedb_object": "document:doc3",
			},
		},
	}

	pipeline := rag.NewRAGPipeline(client, spiceDBTypeDoc, spiceDBPermRead, docs)

	// 5. Run some queries as different users and assert which docs appear.

	// Emilia should see doc1 + doc3, but not doc2.
	{
		results, err := pipeline.Query(ctx, "emilia", "roadmap")
		require.NoError(t, err)
		requireEqualDocIDs(t, []string{"doc1"}, results)

		results, err = pipeline.Query(ctx, "emilia", "public")
		require.NoError(t, err)
		requireEqualDocIDs(t, []string{"doc3"}, results)
	}

	// Beatrice should see doc2 + doc3, but not doc1.
	{
		results, err := pipeline.Query(ctx, "beatrice", "playbook")
		require.NoError(t, err)
		requireEqualDocIDs(t, []string{"doc2"}, results)

		results, err = pipeline.Query(ctx, "beatrice", "public")
		require.NoError(t, err)
		requireEqualDocIDs(t, []string{"doc3"}, results)
	}

	// A random user 'charlie' should only see the public doc3.
	{
		results, err := pipeline.Query(ctx, "charlie", "public")
		require.NoError(t, err)
		requireEqualDocIDs(t, []string{"doc3"}, results)
	}
}

// writeTestSchema configures a tiny SpiceDB schema for documents/users.
func writeTestSchema(t *testing.T, ctx context.Context, client *authzed.Client) {
	t.Helper()

	schema := `
definition user {}

definition document {
  relation owner: user
  relation viewer: user

  permission read = owner + viewer
}
`
	_, err := client.WriteSchema(ctx, &apiv1.WriteSchemaRequest{
		Schema: schema,
	})
	require.NoError(t, err, "failed to write schema")
}

// writeTestTuples seeds a few relationships in SpiceDB:
// - Emilia owns doc1
// - Beatrice can view doc2
// - Everyone can view doc3 (via viewer).
func writeTestTuples(t *testing.T, ctx context.Context, client *authzed.Client) {
	t.Helper()

	var updates []*apiv1.RelationshipUpdate

	// Emilia owns doc1
	updates = append(updates, relUpdate(
		spiceDBTypeDoc, "doc1",
		"owner",
		"user", "emilia",
	))

	// Beatrice views doc2
	updates = append(updates, relUpdate(
		spiceDBTypeDoc, "doc2",
		"viewer",
		"user", "beatrice",
	))

	// Everyone can view doc3 (viewer for emilia + beatrice + charlie)
	for _, userID := range []string{"emilia", "beatrice", "charlie"} {
		updates = append(updates, relUpdate(
			spiceDBTypeDoc, "doc3",
			"viewer",
			"user", userID,
		))
	}

	_, err := client.WriteRelationships(ctx, &apiv1.WriteRelationshipsRequest{
		Updates: updates,
	})
	require.NoError(t, err, "failed to write relationships")
}

func relUpdate(resType, resID, relation, subjType, subjID string) *apiv1.RelationshipUpdate {
	return &apiv1.RelationshipUpdate{
		Operation: apiv1.RelationshipUpdate_OPERATION_CREATE,
		Relationship: &apiv1.Relationship{
			Resource: &apiv1.ObjectReference{
				ObjectType: resType,
				ObjectId:   resID,
			},
			Relation: relation,
			Subject: &apiv1.SubjectReference{
				Object: &apiv1.ObjectReference{
					ObjectType: subjType,
					ObjectId:   subjID,
				},
			},
		},
	}
}

// requireEqualDocIDs is a tiny helper that asserts the returned documents
// have exactly these IDs (order-insensitive).
func requireEqualDocIDs(t *testing.T, expected []string, docs []rag.Document) {
	t.Helper()

	got := make(map[string]struct{}, len(docs))
	for _, d := range docs {
		got[d.ID] = struct{}{}
	}

	if len(got) != len(expected) {
		t.Fatalf("expected %d docs, got %d (got: %+v)", len(expected), len(got), got)
	}

	for _, id := range expected {
		if _, ok := got[id]; !ok {
			t.Fatalf("expected doc %q in results, but it was missing; got: %+v", id, got)
		}
	}
}
