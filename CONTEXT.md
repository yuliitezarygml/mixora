# Mixora music personalization

Mixora lets one listener find, play, organise and receive recommendations for
music supplied by external sources. This glossary fixes the product words used
by the client, API and future work.

## Music

**Music engine**:
The existing source-facing service that finds music and supplies playable track
data. It does not own a listener's account, collection or recommendations.
_Avoid_: catalog backend, user backend

**Track reference**:
The stable source-and-identifier pair that names one track independently of
where it is displayed.
_Avoid_: Spotify ID, SoundCloud ID, track URL

**Track snapshot**:
The small renderable description of a track retained with a listener action.
It is not a replacement for the full source catalogue record.
_Avoid_: track cache, track copy

## Listener library

**Account library**:
The collection of music-related state belonging to one Mixora account.
_Avoid_: browser library, local collection

**Track preference**:
The listener's current explicit relationship with a track: liked, disliked or
neutral. It is a current choice, not a list of historical reactions.
_Avoid_: like event, dislike event

**Listening history**:
The listener's recent distinct tracks together with their listening aggregate.
It represents what was heard, not every raw player event.
_Avoid_: event log, play queue

**Playlist**:
An account-owned, named collection of tracks deliberately arranged by its
owner. A playlist from a music source is an external catalogue object, not a
Mixora playlist.
_Avoid_: source playlist, album

**Playlist entry**:
One track included in one playlist. A track occurs at most once in a given
playlist.
_Avoid_: duplicate track, queue item

**Playlist order**:
The owner-defined sequence of playlist entries. Sorting for display never
changes this sequence.
_Avoid_: alphabetical order, playback queue

**Legacy library snapshot**:
The transitional browser-compatible representation of account-library data.
It is not the authority for normalized preferences, history or playlists.
_Avoid_: account library, source of truth
