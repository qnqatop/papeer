package app

import (
	"sort"
	"time"

	"github.com/qnqatop/papeer/internal/db"
	"github.com/qnqatop/papeer/internal/graphalgo"
)

// keyPapersLimit caps each ranking list returned by GetKeyPapers.
const keyPapersLimit = 10

// KeyPaper is a paper annotated with the score that earned it a spot in one
// of the three Key Papers rankings (citation count, citation velocity, or
// betweenness centrality — the meaning of Score depends on which list it's
// in, see KeyPapersResult).
type KeyPaper struct {
	db.Paper
	Score float64 `json:"score"`
}

// KeyPapersResult holds the three "what to read first" rankings surfaced by
// the Key Papers tab.
type KeyPapersResult struct {
	// MostCited: plain citation_count desc.
	MostCited []KeyPaper `json:"most_cited"`
	// Rising: citation_count / (current_year - pub_year + 1) — citation
	// velocity, surfaces recent papers accumulating citations fast even if
	// their raw count is still low.
	Rising []KeyPaper `json:"rising"`
	// Bridge: betweenness centrality on the internal citation graph —
	// papers that connect otherwise-separate parts of the corpus.
	Bridge []KeyPaper `json:"bridge"`
}

// GetKeyPapers computes the three Key Papers rankings for a profile's
// approved/downloaded papers: Most Cited, Rising (citation velocity), and
// Bridge (Brandes betweenness centrality over citation_links). All three run
// entirely locally over already-fetched data (citations must have been
// fetched at least once for Bridge to return anything).
func (a *App) GetKeyPapers(profileID int64) (*KeyPapersResult, error) {
	papers, err := a.approvedAndDownloadedFull(profileID)
	if err != nil {
		return nil, err
	}
	if len(papers) == 0 {
		return &KeyPapersResult{}, nil
	}

	mostCited := make([]KeyPaper, len(papers))
	for i, p := range papers {
		mostCited[i] = KeyPaper{Paper: p, Score: float64(p.CitationCount)}
	}
	sort.Slice(mostCited, func(i, j int) bool { return mostCited[i].Score > mostCited[j].Score })
	mostCited = capKeyPapers(mostCited, keyPapersLimit)

	curYear := time.Now().Year()
	var rising []KeyPaper
	for _, p := range papers {
		if p.Year == nil {
			continue
		}
		age := curYear - *p.Year + 1
		if age < 1 {
			age = 1
		}
		rising = append(rising, KeyPaper{Paper: p, Score: float64(p.CitationCount) / float64(age)})
	}
	sort.Slice(rising, func(i, j int) bool { return rising[i].Score > rising[j].Score })
	rising = capKeyPapers(rising, keyPapersLimit)

	bridge, err := a.rankByBetweenness(profileID, papers)
	if err != nil {
		return nil, err
	}

	return &KeyPapersResult{MostCited: mostCited, Rising: rising, Bridge: bridge}, nil
}

func (a *App) rankByBetweenness(profileID int64, papers []db.Paper) ([]KeyPaper, error) {
	links, err := a.db.GetCitationLinks(profileID)
	if err != nil {
		return nil, err
	}
	if len(links) == 0 {
		return nil, nil
	}

	nodeSet := make(map[int64]bool)
	adj := make(map[int64][]int64)
	for _, l := range links {
		nodeSet[l.FromPaperID] = true
		nodeSet[l.ToPaperID] = true
		adj[l.FromPaperID] = append(adj[l.FromPaperID], l.ToPaperID)
		adj[l.ToPaperID] = append(adj[l.ToPaperID], l.FromPaperID)
	}
	nodes := make([]int64, 0, len(nodeSet))
	for id := range nodeSet {
		nodes = append(nodes, id)
	}

	centrality := graphalgo.Betweenness(nodes, adj)

	byID := make(map[int64]db.Paper, len(papers))
	for _, p := range papers {
		byID[p.ID] = p
	}

	var bridge []KeyPaper
	for id, score := range centrality {
		if score <= 0 {
			continue
		}
		p, ok := byID[id]
		if !ok {
			continue // link references a paper outside the current eligible set
		}
		bridge = append(bridge, KeyPaper{Paper: p, Score: score})
	}
	sort.Slice(bridge, func(i, j int) bool {
		if bridge[i].Score != bridge[j].Score {
			return bridge[i].Score > bridge[j].Score
		}
		return bridge[i].ID < bridge[j].ID
	})
	return capKeyPapers(bridge, keyPapersLimit), nil
}

func capKeyPapers(list []KeyPaper, limit int) []KeyPaper {
	if len(list) > limit {
		return list[:limit]
	}
	return list
}
