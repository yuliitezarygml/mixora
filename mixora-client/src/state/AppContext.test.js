import assert from "node:assert/strict";
import { after, test } from "node:test";
import { act, createElement, useContext } from "react";
import { createRoot } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import { JSDOM } from "jsdom";
import { createServer } from "vite";

const server = await createServer({
  server: { middlewareMode: true },
  appType: "custom",
});
after(() => server.close());
const { AppProvider } = await server.ssrLoadModule("/src/state/AppContext.jsx");
const { AppContext } = await server.ssrLoadModule("/src/state/context.js");
const { PlayerPanel } = await server.ssrLoadModule(
  "/src/components/Player.jsx",
);
const { default: RecommendedPlaylists } = await server.ssrLoadModule(
  "/src/components/RecommendedPlaylists.jsx",
);
const { TrackList } = await server.ssrLoadModule("/src/components/Tracks.jsx");
const { Cover } = await server.ssrLoadModule("/src/components/Primitives.jsx");
const { default: CollectionOverview } = await server.ssrLoadModule(
  "/src/pages/CollectionOverview.jsx",
);

test("collection uses the reference heading, actual like count and artists before recommendations", () => {
  const markup = renderToStaticMarkup(
    createElement(
      AppContext.Provider,
      {
        value: {
          user: { id: "listener" },
          current: null,
          settings: { explicit: true },
          library: {
            likes: [
              {
                id: "one",
                source: "youtube",
                title: "Real song",
                artist: "Real artist",
              },
            ],
            artists: [],
            history: [],
            listens: [],
            playlists: [],
            savedPlaylists: [],
            albums: [],
            pins: [],
          },
          recommendations: { playlists: [], loading: false, failures: 0 },
        },
      },
      createElement(MemoryRouter, null, createElement(CollectionOverview)),
    ),
  );
  assert.match(markup, /У вашей музыки есть/);
  assert.match(markup, /heart\.602389ae\.png/);
  assert.match(markup, /1 трек/);
  assert.ok(
    markup.indexOf("collection-artists-title") <
      markup.indexOf("Рекомендуемые плейлисты"),
  );
  assert.ok(!markup.includes("artist-rank"));
  assert.ok(markup.includes('class="collection-favorite-tracks"'));
  assert.ok(!markup.includes('class="collection-likes"'));
});

test("recommended playlists render safely before the account is loaded", () => {
  const markup = renderToStaticMarkup(
    createElement(
      AppContext.Provider,
      {
        value: {
          user: null,
          library: {},
          settings: { explicit: true },
          recommendations: { playlists: [], loading: false, failures: 0 },
        },
      },
      createElement(RecommendedPlaylists),
    ),
  );
  assert.match(markup, /Войти для персональных подборок/);
});

test("recommendation track previews display all five sources and Spotify fragment access", () => {
  const sources = ["soundcloud", "youtube", "vk", "bandcamp", "spotify"];
  const markup = renderToStaticMarkup(
    createElement(
      AppContext.Provider,
      {
        value: {
          current: null,
          library: { likes: [] },
          settings: { explicit: true },
        },
      },
      createElement(
        MemoryRouter,
        null,
        createElement(TrackList, {
          tracks: sources.map((source) => ({
            source,
            id: "one",
            title: "Music",
            artist: "Artist",
            access: source === "spotify" ? "preview" : "playable",
          })),
          compact: true,
          showSourceInMetadata: true,
        }),
      ),
    ),
  );
  for (const name of [
    "SoundCloud",
    "YouTube",
    "VK",
    "Bandcamp",
    "Spotify · Фрагмент",
  ])
    assert.ok(markup.includes(name), name);
});

const user = { id: "listener", email: "listener@example.test" };
const tracks = ["a", "b", "c", "d"].map((id) => ({
  id,
  source: "youtube",
  title: `Track ${id}`,
  artist: "Artist",
  permalink: `https://www.youtube.com/watch?v=${id}`,
}));
const sessionId = "123e4567-e89b-12d3-a456-426614174100";
const playerKey = `mixora-ui:player:${user.id}`;

async function mount(
  t,
  {
    savedPlayer,
    audioReadyState = 1,
    loginUser = user,
    renderPanel = false,
    renderCover = false,
  } = {},
) {
  const dom = new JSDOM('<div id="root"></div>', {
    url: "http://127.0.0.1:5174/",
  });
  const sockets = [];
  const audioElements = [];
  class Socket {
    readyState = 1;
    messages = [];
    constructor() {
      sockets.push(this);
    }
    send(message) {
      this.messages.push(JSON.parse(message));
    }
    close() {
      this.readyState = 3;
      this.onclose?.();
    }
  }
  class Audio extends dom.window.EventTarget {
    currentTime = 0;
    duration = 240;
    readyState = audioReadyState;
    paused = true;
    src = "";
    constructor() {
      super();
      audioElements.push(this);
    }
    play() {
      this.paused = false;
      this.dispatchEvent(new dom.window.Event("play"));
      return Promise.resolve();
    }
    pause() {
      if (this.paused) return;
      this.paused = true;
      this.dispatchEvent(new dom.window.Event("pause"));
    }
    removeAttribute() {
      this.src = "";
    }
    load() {
      this.currentTime = 0;
    }
  }
  const replacements = {
    window: dom.window,
    document: dom.window.document,
    navigator: dom.window.navigator,
    localStorage: dom.window.localStorage,
    Audio,
    WebSocket: Socket,
    IS_REACT_ACT_ENVIRONMENT: true,
  };
  const originals = new Map();
  for (const [key, value] of Object.entries(replacements)) {
    originals.set(key, Object.getOwnPropertyDescriptor(globalThis, key));
    Object.defineProperty(globalThis, key, {
      configurable: true,
      writable: true,
      value,
    });
  }
  if (savedPlayer) localStorage.setItem(playerKey, JSON.stringify(savedPlayer));
  const requests = [];
  const preferences = new Map();
  t.mock.method(globalThis, "fetch", async (path, options = {}) => {
    const body = options.body ? JSON.parse(options.body) : null;
    requests.push({ path, body, method: options.method || "GET" });
    let value = {};
    if (path === "/api/v1/me" || path === "/api/v1/auth/session") value = user;
    if (path === "/api/v1/auth/login") value = loginUser;
    if (path === "/api/v1/me/history") value = { entries: [], generation: 0 };
    if (path === "/api/v1/me/playlists") value = { playlists: [] };
    if (path.startsWith("/api/v1/me/playlists/") && options.method === "PUT") {
      value = { ...body, id: path.split("/").at(-1), revision: 1 };
    }
    if (path === "/api/v1/me/track-preferences") {
      if (body) {
        value = { track: body.track, preference: body.preference, revision: 1 };
        preferences.set(`${body.track.source}:${body.track.id}`, value);
      } else value = { preferences: [...preferences.values()] };
    }
    if (path === "/api/v1/wave") {
      value = { tracks, session_id: sessionId, model_version: "test-v1" };
    }
    return new Response(JSON.stringify(value), {
      headers: { "Content-Type": "application/json" },
    });
  });
  let state;
  function Capture() {
    state = useContext(AppContext);
    return null;
  }
  const root = createRoot(document.getElementById("root"));
  await act(async () =>
    root.render(
      createElement(
        AppProvider,
        null,
        createElement(Capture),
        renderPanel && createElement(PlayerPanel),
        renderCover &&
          createElement(Cover, {
            track: { artwork: "https://invalid.example.test/cover.jpg" },
          }),
      ),
    ),
  );
  assert.equal(state.user?.id, user.id);
  let unmounted = false;
  const unmount = async () => {
    if (unmounted) return;
    unmounted = true;
    await act(async () => root.unmount());
  };
  t.after(async () => {
    await unmount();
    dom.window.close();
    for (const [key, descriptor] of originals) {
      if (descriptor) Object.defineProperty(globalThis, key, descriptor);
      else delete globalThis[key];
    }
  });
  return { state: () => state, requests, sockets, audioElements, dom, unmount };
}

test("queue buttons move, remove and pause tracks through the rendered UI", async (t) => {
  const app = await mount(t, { renderPanel: true });
  await act(async () => {
    app.state().play(tracks[0], tracks);
    app.state().setPanel("queue");
  });
  const click = async (label) => {
    const button = document.querySelector(`button[aria-label="${label}"]`);
    assert.ok(button, `queue control ${label} should exist`);
    await act(async () =>
      button.dispatchEvent(
        new app.dom.window.MouseEvent("click", { bubbles: true }),
      ),
    );
  };
  await click("Ниже в очереди: Track c");
  assert.deepEqual(
    app.state().queue.map((track) => track.id),
    ["a", "b", "d", "c"],
  );
  await click("Удалить из очереди: Track b");
  assert.deepEqual(
    app.state().queue.map((track) => track.id),
    ["a", "d", "c"],
  );
  assert.equal(
    document.querySelector('button[aria-label="Удалить из очереди: Track a"]')
      .disabled,
    true,
  );
  await click("Приостановить: Track a");
  assert.equal(app.state().playing, false);
});

test("a failed artwork renders a visible fallback instead of an empty image", async (t) => {
  const app = await mount(t, { renderCover: true });
  await act(async () =>
    document
      .querySelector("img.cover")
      .dispatchEvent(new app.dom.window.Event("error")),
  );
  assert.ok(document.querySelector(".cover.fallback svg"));
  assert.equal(document.querySelector("img.cover"), null);
});

test("recommendation cards use bundled Mixora artwork rather than a remote album cover", () => {
  const markup = renderToStaticMarkup(
    createElement(
      AppContext.Provider,
      {
        value: {
          user,
          library: {},
          settings: { explicit: true },
          recommendations: {
            loading: false,
            failures: 0,
            playlists: [
              {
                id: "daily",
                name: "Для вас",
                tracks: [
                  {
                    ...tracks[0],
                    artwork: "https://invalid.example.test/cover.jpg",
                  },
                ],
              },
            ],
          },
        },
      },
      createElement(RecommendedPlaylists),
    ),
  );
  assert.ok(markup.includes("vibe_animation_fallback_dark.jpeg"));
  assert.ok(!markup.includes("invalid.example.test"));
});

test("a restored position survives closing before metadata loads", async (t) => {
  const app = await mount(t, {
    savedPlayer: { track: tracks[0], queue: tracks, index: 0, position: 17.3 },
    audioReadyState: 0,
  });
  window.dispatchEvent(new app.dom.window.Event("pagehide"));
  assert.equal(JSON.parse(localStorage.getItem(playerKey)).position, 17.3);
  await act(async () => app.state().next());
  await act(async () =>
    app.audioElements[0].dispatchEvent(
      new app.dom.window.Event("loadedmetadata"),
    ),
  );
  assert.equal(app.state().current.id, "b");
  assert.equal(app.state().position, 0);
});

test("switching accounts never overwrites the new account with the previous queue", async (t) => {
  const secondUser = { id: "second", email: "second@example.test" };
  const app = await mount(t, { loginUser: secondUser });
  await act(async () => app.state().play(tracks[0], tracks));
  const secondKey = `mixora-ui:player:${secondUser.id}`;
  localStorage.setItem(
    secondKey,
    JSON.stringify({
      track: tracks[3],
      queue: [tracks[3]],
      index: 0,
      position: 32,
    }),
  );
  await act(async () => app.state().login(secondUser.email, "password"));
  assert.equal(app.state().current.id, "d");
  assert.equal(app.state().position, 32);
  window.dispatchEvent(new app.dom.window.Event("pagehide"));
  assert.equal(JSON.parse(localStorage.getItem(secondKey)).track.id, "d");
  assert.equal(JSON.parse(localStorage.getItem(secondKey)).position, 32);
  assert.equal(JSON.parse(localStorage.getItem(playerKey)).track.id, "a");
});

test("a remote reorder updates the queue while the same track keeps playing", async (t) => {
  const app = await mount(t);
  await act(async () => app.state().play(tracks[0], tracks));
  const socket = app.sockets.at(-1);
  const before = socket.messages.length;
  await act(async () =>
    socket.onmessage({
      data: JSON.stringify({
        type: "state",
        track: tracks[0],
        queue: [tracks[0], tracks[2], tracks[1]],
        position: 0,
        playing: true,
      }),
    }),
  );
  assert.deepEqual(
    app.state().queue.map((track) => track.id),
    ["a", "c", "b"],
  );
  assert.equal(app.state().playing, true);
  assert.equal(socket.messages.length, before);
});

test("a remote paused track retains its requested position after source resolution", async (t) => {
  const app = await mount(t);
  await act(async () => app.state().play(tracks[0], tracks));
  await act(async () =>
    app.sockets.at(-1).onmessage({
      data: JSON.stringify({
        type: "state",
        track: tracks[1],
        queue: tracks,
        position: 61,
        playing: false,
      }),
    }),
  );
  assert.equal(app.state().current.id, "b");
  assert.equal(app.state().playing, false);
  assert.equal(app.state().position, 61);
  assert.equal(JSON.parse(localStorage.getItem(playerKey)).position, 61);
});

test("previous, repeat and shuffle preserve playable queue behavior", async (t) => {
  const app = await mount(t);
  await act(async () => app.state().play(tracks[1], tracks));
  await act(async () => app.state().seek(10));
  await act(async () => app.state().previous());
  assert.equal(app.state().current.id, "b");
  await act(async () => app.state().previous());
  assert.equal(app.state().current.id, "a");
  await act(async () => app.state().setRepeat("one"));
  await act(async () => app.state().next(true));
  assert.equal(app.state().current.id, "a");
  await act(async () => app.state().setRepeat("all"));
  await act(async () => app.state().play(tracks[3], tracks));
  await act(async () => app.state().next(true));
  assert.equal(app.state().current.id, "a");
  await act(async () => app.state().toggleShuffle());
  assert.equal(app.state().current.id, "a");
  assert.equal(app.state().index, 0);
  assert.equal(app.state().shuffled, true);
  assert.deepEqual(
    app
      .state()
      .queue.map((track) => track.id)
      .sort(),
    ["a", "b", "c", "d"],
  );
});

test("reordering upcoming tracks persists and syncs without changing queue length", async (t) => {
  const app = await mount(t);
  await act(async () => app.state().play(tracks[0], tracks));
  await act(async () => app.state().moveQueue(2, 3));
  const saved = JSON.parse(localStorage.getItem(playerKey));
  assert.deepEqual(
    saved.queue.map((track) => track.id),
    ["a", "b", "d", "c"],
  );
  assert.deepEqual(
    app.sockets
      .at(-1)
      .messages.at(-1)
      .queue.map((track) => track.id),
    ["a", "b", "d", "c"],
  );
});

test("next is durable before the old 500ms save delay", async (t) => {
  const app = await mount(t);
  await act(async () => app.state().play(tracks[0], tracks));
  await act(async () => app.state().next());
  await app.unmount();
  assert.equal(JSON.parse(localStorage.getItem(playerKey)).track.id, "b");
});

test("pagehide saves the latest position for a paused restore", async (t) => {
  const app = await mount(t, {
    savedPlayer: { track: tracks[0], queue: tracks, index: 0, position: 17.3 },
  });
  assert.equal(app.state().current.id, "a");
  assert.equal(app.state().playing, false);
  await act(async () => app.state().seek(34.2));
  window.dispatchEvent(new app.dom.window.Event("pagehide"));
  assert.equal(JSON.parse(localStorage.getItem(playerKey)).position, 34.2);
});

test("Wave like and dislike deliver immediate session feedback", async (t) => {
  const app = await mount(t);
  await act(async () => app.state().startWave());
  await act(async () => app.state().toggleLike(tracks[0]));
  await act(async () => app.state().dislike(tracks[1]));
  assert.deepEqual(
    app.requests
      .filter(
        (request) => request.path === `/api/v1/wave/${sessionId}/feedback`,
      )
      .map((request) => request.body.type)
      .filter((type) => type === "like" || type === "dislike"),
    ["like", "dislike"],
  );
});

test("recommended playback keeps session feedback and refuses a different account's mix", async (t) => {
  const app = await mount(t);
  const playlist = { id: "daily", owner: user.id, tracks, sessionId };
  await act(async () =>
    app.state().playRecommendedPlaylist(playlist, tracks[1]),
  );
  assert.equal(app.state().current.id, "b");
  assert.equal(app.state().queue.length, 4);
  await act(async () => app.state().toggleLike(tracks[1]));
  assert.ok(
    app.requests.some(
      (request) =>
        request.path === `/api/v1/wave/${sessionId}/feedback` &&
        request.body.type === "like",
    ),
  );
  await act(async () =>
    app
      .state()
      .playRecommendedPlaylist({ ...playlist, owner: "other" }, tracks[3]),
  );
  assert.equal(app.state().current.id, "b");
});

test("previewing a recommendation attributes likes and playlist saves before playback", async (t) => {
  const app = await mount(t);
  const playlist = { id: "daily", owner: user.id, tracks, sessionId };
  await act(async () => app.state().rememberRecommendedPlaylist(playlist));
  await act(async () => app.state().toggleLike(tracks[0]));
  await act(async () => app.state().createPlaylist("Для вас", tracks));
  assert.equal(app.state().library.playlists[0].tracks.length, 4);
  const feedback = app.requests.filter(
    (request) => request.path === `/api/v1/wave/${sessionId}/feedback`,
  );
  assert.ok(feedback.some((request) => request.body.type === "like"));
  assert.equal(
    feedback.filter((request) => request.body.type === "add_to_playlist")
      .length,
    4,
  );
  assert.equal(app.state().playing, false);
});

test("a full localStorage reports an unsaved like and sends no preference or feedback", async (t) => {
  const app = await mount(t);
  await act(async () => app.state().startWave());
  const before = app.requests.length;
  t.mock.method(app.dom.window.Storage.prototype, "setItem", () => {
    throw new Error("storage full");
  });
  await act(async () => app.state().toggleLike(tracks[0]));
  assert.equal(app.state().library.likes.length, 0);
  assert.match(app.state().notice, /Не удалось сохранить/);
  assert.equal(
    app.requests
      .slice(before)
      .some(
        (request) => request.method === "PUT" || request.body?.type === "like",
      ),
    false,
  );
});
