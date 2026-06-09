package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
)

type fakeTagSearchRepo struct {
	keyword string
	limit   int
	tags    []model.Tag
	err     error
}

func (r *fakeTagSearchRepo) Search(keyword string, limit int) ([]model.Tag, error) {
	r.keyword = keyword
	r.limit = limit
	return r.tags, r.err
}

type fakeArticleSearchRepo struct {
	opts repository.ListArticleTagOpts
	docs []model.ArticleTag
	err  error
}

func (r *fakeArticleSearchRepo) ListWithFilter(opts repository.ListArticleTagOpts) ([]model.ArticleTag, int64, error) {
	r.opts = opts
	return r.docs, int64(len(r.docs)), r.err
}

type fakeRSSSearchRepo struct {
	keyword string
	limit   int
	feeds   []model.RSSFeed
	err     error
}

func (r *fakeRSSSearchRepo) Search(keyword string, limit int) ([]model.RSSFeed, error) {
	r.keyword = keyword
	r.limit = limit
	return r.feeds, r.err
}

func TestSearchServiceSearchAllAggregatesResults(t *testing.T) {
	tags := &fakeTagSearchRepo{tags: []model.Tag{{ID: 1, Name: "go"}}}
	docs := &fakeArticleSearchRepo{docs: []model.ArticleTag{{ID: 2, ArticleTitle: "Go notes"}}}
	rss := &fakeRSSSearchRepo{feeds: []model.RSSFeed{{ID: 3, Name: "Go feed"}}}
	svc := NewSearchService(tags, docs, rss)

	result, err := svc.Search(" go ", "all", 0)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if result.TotalCount != 3 {
		t.Fatalf("TotalCount = %d, want 3", result.TotalCount)
	}
	if tags.keyword != "%go%" || tags.limit != 10 {
		t.Fatalf("tag search keyword/limit = %q/%d, want %%go%%/10", tags.keyword, tags.limit)
	}
	if docs.opts.Keyword != "go" || docs.opts.PerPage != 10 {
		t.Fatalf("document search opts = %+v, want keyword go and per_page 10", docs.opts)
	}
	if rss.keyword != "%go%" || rss.limit != 10 {
		t.Fatalf("rss search keyword/limit = %q/%d, want %%go%%/10", rss.keyword, rss.limit)
	}
}

func TestSearchServiceScopeOnlyCallsRequestedRepo(t *testing.T) {
	tags := &fakeTagSearchRepo{tags: []model.Tag{{ID: 1, Name: "go"}}}
	docs := &fakeArticleSearchRepo{}
	rss := &fakeRSSSearchRepo{}
	svc := NewSearchService(tags, docs, rss)

	result, err := svc.Search("go", "tags", 5)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(result.Tags) != 1 || len(result.Documents) != 0 || len(result.RSSFeeds) != 0 {
		t.Fatalf("unexpected scoped result: %+v", result)
	}
	if docs.opts.PerPage != 0 || rss.limit != 0 {
		t.Fatalf("non-tag repos should not be called, docs=%+v rss_limit=%d", docs.opts, rss.limit)
	}
}

func TestSearchServicePropagatesRepoError(t *testing.T) {
	svc := NewSearchService(
		&fakeTagSearchRepo{err: errors.New("db down")},
		&fakeArticleSearchRepo{},
		&fakeRSSSearchRepo{},
	)

	_, err := svc.Search("go", "tags", 5)
	if err == nil || !strings.Contains(err.Error(), "search tags") {
		t.Fatalf("Search err = %v, want wrapped tag error", err)
	}
}

func TestSearchServiceRejectsInvalidScope(t *testing.T) {
	svc := NewSearchService(&fakeTagSearchRepo{}, &fakeArticleSearchRepo{}, &fakeRSSSearchRepo{})

	_, err := svc.Search("go", "unknown", 5)
	if err == nil || !strings.Contains(err.Error(), "invalid search scope") {
		t.Fatalf("Search err = %v, want invalid scope", err)
	}
}
