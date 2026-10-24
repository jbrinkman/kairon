package boundary

import (
	"context"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/provenance"
	"github.com/jbrinkman/kairon/internal/stageoutput"
)

// TestRoundTrip_CommitThenAdmitBodyMatchesHashedBody proves the reader and the
// writer agree: what provenance.Committer writes is what Admit verifies, and the
// document carries exactly the canonical body and hash the committer produced.
func TestRoundTrip_CommitThenAdmitBodyMatchesHashedBody(t *testing.T) {
	variants := map[string]string{
		"LF":   "# Title\n\nDo the thing.   \n\nDependencies: #314\n\n",
		"CRLF": "# Title\r\n\r\nDo the thing.   \r\n\r\nDependencies: #314\r\n\r\n",
	}
	for name, planned := range variants {
		t.Run(name, func(t *testing.T) {
			store := newFakeStore()
			c := &provenance.Committer{Store: store, Now: func() time.Time { return fixedTime }}
			res, err := c.Commit(context.Background(), provenance.Request{
				Doc: &stageoutput.Document{
					SchemaVersion: 1,
					Stage:         stageoutput.StagePlanner,
					Result:        stageoutput.ResultCanonical,
					Source:        stageoutput.Source{Kind: stageoutput.SourceGitHubIssue, Ref: "owner/name#382"},
					Generated:     "2026-03-04T05:06:07Z",
					Body:          planned,
				},
				Title:     "Round trip",
				Admission: guard.Result{Outcome: guard.OutcomeReadyToCommit},
			})
			if err != nil {
				t.Fatalf("Commit: %v", err)
			}
			written := store.bodies[res.Number]
			if written != res.Body {
				t.Fatal("test setup: store body differs from Result.Body")
			}

			// The reader returns exactly the text the committer wrote.
			r := &fakeReader{snapshots: []Snapshot{openSnapshot(written)}}
			adm := mustAdmit(t, newBoundary(r, nil))
			if adm.Outcome != OutcomeAdmitted {
				t.Fatalf("Outcome = %v (%+v), want the committer's output to be admitted", adm.Outcome, adm.Failure)
			}
			doc := adm.Document

			if doc.Body != provenance.Canonicalize(written) {
				t.Errorf("Body = %q, want Canonicalize(written) = %q", doc.Body, provenance.Canonicalize(written))
			}
			if doc.Body != provenance.Canonicalize(planned) {
				t.Errorf("Body = %q, want the planner's canonical body %q", doc.Body, provenance.Canonicalize(planned))
			}
			if got := provenance.Hash(doc.Body); got != doc.Provenance.SHA256 || got != res.Hash {
				t.Errorf("Hash(Body) = %q, Provenance.SHA256 = %q, committed Hash = %q; all three must agree",
					got, doc.Provenance.SHA256, res.Hash)
			}

			// Marshal then Parse returns identical content.
			data, err := doc.Marshal()
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			parsed, err := ParseInputDocument(data)
			if err != nil {
				t.Fatalf("ParseInputDocument: %v", err)
			}
			if *parsed != *doc {
				t.Errorf("parsed document differs:\n got %+v\nwant %+v", *parsed, *doc)
			}
			if parsed.Body != doc.Body {
				t.Error("Body bytes changed across Marshal/Parse")
			}
			data2, err := parsed.Marshal()
			if err != nil || string(data2) != string(data) {
				t.Errorf("second Marshal differs (err %v)", err)
			}
		})
	}
}
