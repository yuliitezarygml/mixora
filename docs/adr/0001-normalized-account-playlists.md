# Account playlists are normalized state, not a library snapshot

Status: accepted. Mixora owns account playlists and their ordered entries as
first-class account state, while the existing music engine remains responsible
only for source music. This is deliberately different from the transitional
library snapshot: it makes playlist ownership, ordering and multi-device
behaviour explicit without duplicating the music engine or treating external
source playlists as editable Mixora objects.

## Consequences

Playlist writes preserve a compact track snapshot and a stable track reference;
source metadata can still be refreshed independently. The snapshot remains a
compatibility cache during migration and must not overwrite the normalized
playlist state.
