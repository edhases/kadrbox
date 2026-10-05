Title: Search is resilient to provider exceptions

Given:
- Provider A returns results for 'query'
- Provider B throws Exception when called

When:
- Call UnifiedContentRepository.search('query')

Mocks/Helpers:
- Mock provider implementations; provider B set to throw on search

Expectations:
- The search result contains provider A's items only
- No exception is propagated to the caller
- A warning is logged for provider B failure

Notes:
- This test ensures partial failures don't crash search UI; UI should show results available and allow retrying failed provider
