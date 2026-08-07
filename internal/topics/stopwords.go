package topics

// stopwords combines standard English stop words with academic-boilerplate
// terms that are frequent across almost any research corpus and therefore
// useless for distinguishing topics ("this paper proposes a novel method to
// analyze..." appears in half of all abstracts). Without this filter,
// K-Means cluster labels devolve into "Novel / Method / Approach" for every
// cluster — this list is what keeps labels meaningful.
var stopwords = buildStopwords()

func buildStopwords() map[string]bool {
	words := []string{
		// Standard English stop words.
		"the", "and", "for", "are", "but", "not", "you", "all", "can", "her",
		"was", "one", "our", "out", "has", "have", "had", "his", "him", "its",
		"who", "did", "yet", "few", "how", "why", "than", "then", "them",
		"they", "this", "that", "these", "those", "with", "from", "into",
		"over", "under", "about", "after", "before", "between", "during",
		"through", "while", "where", "when", "which", "what", "will", "would",
		"could", "should", "shall", "may", "might", "must", "also", "such",
		"some", "any", "each", "both", "more", "most", "other", "same", "own",
		"than", "too", "very", "just", "only", "here", "there", "again",
		"further", "once", "does", "doing", "been", "being", "were", "itself",
		"themselves", "ourselves", "yourself", "because", "against", "above",
		"below", "off", "onto", "upon", "within", "without", "per", "via",
		"across", "among", "along", "toward", "towards", "thus", "hence",
		"however", "therefore", "moreover", "furthermore", "although",
		"though", "either", "neither", "whether", "given", "namely",
		"including", "include", "includes", "included",

		// Academic/scientific boilerplate — near-universal in abstracts and
		// therefore not distinguishing between topics.
		"study", "studies", "studied", "analysis", "analyses", "analyze",
		"analyzed", "analyzing", "model", "models", "modeling", "modelled",
		"method", "methods", "methodology", "result", "results", "resulting",
		"approach", "approaches", "paper", "papers", "using", "used", "use",
		"uses", "based", "novel", "propose", "proposed", "proposes",
		"proposing", "present", "presents", "presented", "presenting", "work",
		"framework", "frameworks", "system", "systems", "dataset", "datasets",
		"data", "performance", "experiment", "experiments", "experimental",
		"evaluate", "evaluated", "evaluating", "evaluation", "show", "shows",
		"showed", "shown", "showing", "demonstrate", "demonstrates",
		"demonstrated", "demonstrating", "achieve", "achieves", "achieved",
		"achieving", "compared", "comparison", "compare", "comparing",
		"significant", "significantly", "important", "existing", "recent",
		"recently", "state", "art", "provide", "provides", "provided",
		"providing", "introduce", "introduces", "introduced", "introducing",
		"consider", "considered", "considering", "various", "several",
		"different", "large", "small", "high", "low", "new", "well", "many",
		"paper's", "research", "literature", "review", "field", "fields",
		"problem", "problems", "task", "tasks", "application", "applications",
		"case", "cases", "aim", "aims", "aimed", "order", "terms", "term",
		"number", "numbers", "set", "sets", "value", "values", "level",
		"levels", "based", "purpose", "conclusion", "conclusions", "findings",
		"finding", "discuss", "discussed", "discussion", "described",
		"describe", "describes", "describing",
	}
	out := make(map[string]bool, len(words))
	for _, w := range words {
		out[w] = true
	}
	return out
}
