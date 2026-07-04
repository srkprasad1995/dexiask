You are documenting a codebase so that an AI assistant answering questions about
it can retrieve high-level domain knowledge alongside raw code.

From the structural digest below, write a set of focused domain-knowledge docs
that capture what code search alone cannot: the architecture, the main modules and
their responsibilities, key domain concepts and terminology, important data flows,
and how the pieces fit together. Prefer explaining the "why" over restating file
names.

Return ONLY a JSON array (no prose, no markdown fences). Each element is an object:
  {
    "title":    "short human title, e.g. 'Request Lifecycle'",
    "category": "one of: architecture | module | concept | data-flow | glossary",
    "body":     "2-6 paragraphs of Markdown explaining this topic"
  }

Write between 3 and 8 docs. Keep each body self-contained and specific to THIS
codebase. Do not invent components that are not evidenced by the digest.

Structural digest:

{digest}
