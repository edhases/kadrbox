Title: Search aggregates and deduplicates results from enabled providers

Given:
- Provider A (enabled) returns items: A1(id: '1' provider:A), A2(id:'2' provider:A)
- Provider B (enabled) returns items: B1(id: '2' provider:B), B2(id:'3' provider:B) — note id '2' duplicates by uniqueId (same providerId:id combination should be considered unique; dedupe uses provider + id or uniqueId?)
- Provider C is disabled and should not be queried

When:
- Call UnifiedContentRepository.search('query')

Mocks/Helpers:
- MockContentProvider implementations returning above lists

Expectations:
- Final results include items with unique uniqueId. If duplicates across providers (same providerId:id) are not possible, ensure dedupe removes identical global unique items.
- Results contain three entries representing union without duplicates: (A1, A2/B1 dedup handling, B2) OR if dedupe works across title equality, expect two items depending on exact dedupe rule (specify exact algorithm in assertions)

Notes:
- Clarify deduplication mechanism in repository: if dedupe key is item.uniqueId (providerId:id), cross-provider duplicates should not occur. If dedupe by title/url is required, add test case accordingly.
