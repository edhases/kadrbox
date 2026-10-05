Title: FavoritesDao.toggle adds and removes favorites and watch stream updates

Given:
- Empty Favorites table in in-memory DB

When:
- Call FavoritesDao.add(mediaId: '1', providerId: 'uakino', title: 'A', mediaType: 'movie')
- Then call FavoritesDao.isFavorite('1','uakino')
- Then call FavoritesDao.toggle same item (or remove)

Mocks/Helpers:
- Use in-memory AppDatabase helper

Expectations:
- After add: isFavorite returns true; count() increases
- After toggle/remove: isFavorite returns false; count() decreased
- Watch favorite stream emits appropriate values in sequence

Notes:
- Test insertion uniqueness (insertOrIgnore) by calling add twice
