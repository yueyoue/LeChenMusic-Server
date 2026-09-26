package persistence

import (
	"strings"
	"testing"

	. "github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/model"
)

// The artist query joins library_artist and library, and `library` also has a `name`
// column. Filters built directly as model.QueryOptions{Filters: Eq{...}} used to be
// dropped into the WHERE clause verbatim, so filtering artists by name failed with
// "ambiguous column name: name". qualifyFilters fixes that by qualifying the columns that
// belong to the repository's own table. These tests pin the rules down.

func newArtistRepoForFilterTest() *sqlRepository {
	r := &sqlRepository{}
	r.tableName = "artist"
	r.registerModel(&model.Artist{}, nil)
	return r
}

func whereSQL(t *testing.T, r *sqlRepository, f Sqlizer) string {
	t.Helper()
	sq := r.applyFilters(Select("*").From(r.tableName), model.QueryOptions{Filters: f})
	sql, _, err := sq.ToSql()
	if err != nil {
		t.Fatalf("ToSql: %v", err)
	}
	return sql
}

func TestQualifyFiltersAddsTableToOwnColumns(t *testing.T) {
	r := newArtistRepoForFilterTest()
	sql := whereSQL(t, r, Eq{"name": "The Beatles"})
	if !strings.Contains(sql, "artist.name = ?") {
		t.Errorf("expected `artist.name` to be qualified, got: %s", sql)
	}
}

func TestQualifyFiltersKeepsAnnotationColumnsUnqualified(t *testing.T) {
	r := newArtistRepoForFilterTest()
	// `starred`/`rating`/... live in the separate `annotation` table (they come from the
	// embedded model.Annotations struct), so they must NOT get the artist. prefix.
	for _, col := range []string{"starred", "starred_at", "rating", "rated_at", "play_count"} {
		sql := whereSQL(t, r, Eq{col: true})
		if strings.Contains(sql, "artist."+col) {
			t.Errorf("annotation column %q must not be qualified, got: %s", col, sql)
		}
		if !strings.Contains(sql, col+" = ?") {
			t.Errorf("column %q missing from query: %s", col, sql)
		}
	}
}

func TestQualifyFiltersLeavesExplicitQualifiersAlone(t *testing.T) {
	r := newArtistRepoForFilterTest()
	for _, col := range []string{"artist.name", "library_artist.library_id", "1"} {
		sql := whereSQL(t, r, Eq{col: "x"})
		if strings.Contains(sql, "artist.artist.") || strings.Contains(sql, "artist.library_artist.") {
			t.Errorf("column %q must not be re-qualified, got: %s", col, sql)
		}
	}
}

func TestQualifyFiltersHandlesNestedConditions(t *testing.T) {
	r := newArtistRepoForFilterTest()
	sql := whereSQL(t, r, And{
		Eq{"name": "Kraftwerk"},
		Or{Eq{"missing": false}, Eq{"artist.name": "X"}},
	})
	for _, want := range []string{"artist.name = ?", "artist.missing = ?"} {
		if !strings.Contains(sql, want) {
			t.Errorf("expected %q in: %s", want, sql)
		}
	}
}

func TestQualifyFiltersHandlesComparisonOperators(t *testing.T) {
	r := newArtistRepoForFilterTest()
	sql := whereSQL(t, r, Gt{"created_at": "2020-01-01"})
	if !strings.Contains(sql, "artist.created_at > ?") {
		t.Errorf("expected `artist.created_at > ?`, got: %s", sql)
	}
}

// A repository without a model registered (no column knowledge) must not touch anything.
func TestQualifyFiltersIsANoOpWithoutColumnKnowledge(t *testing.T) {
	r := &sqlRepository{tableName: "artist"}
	sql := whereSQL(t, r, Eq{"name": "x"})
	if !strings.Contains(sql, "name = ?") || strings.Contains(sql, "artist.name") {
		t.Errorf("expected the column to be left untouched, got: %s", sql)
	}
}
