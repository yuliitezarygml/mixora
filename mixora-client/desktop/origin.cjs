const DESKTOP_HOST = "127.0.0.1";
const DESKTOP_PORT = 5174;
const DESKTOP_ORIGIN = `http://${DESKTOP_HOST}:${DESKTOP_PORT}`;
const DEV_SERVER_HEADER = "x-mixora-dev-server";

function isMixoraDevServerResponse(response) {
  return (
    response.statusCode === 200 && response.headers[DEV_SERVER_HEADER] === "1"
  );
}

function listenOnDesktopOrigin(
  server,
  { host = DESKTOP_HOST, port = DESKTOP_PORT } = {},
) {
  return new Promise((resolve, reject) => {
    const cleanup = () => {
      server.off("error", onError);
      server.off("listening", onListening);
    };
    const onError = (error) => {
      cleanup();
      reject(error);
    };
    const onListening = () => {
      cleanup();
      const address = server.address();
      if (!address || typeof address === "string") {
        reject(new Error("Mixora desktop server has no TCP address"));
        return;
      }
      resolve(`http://${host}:${address.port}`);
    };

    server.once("error", onError);
    server.once("listening", onListening);
    try {
      server.listen(port, host);
    } catch (error) {
      onError(error);
    }
  });
}

module.exports = {
  DESKTOP_HOST,
  DESKTOP_PORT,
  DESKTOP_ORIGIN,
  DEV_SERVER_HEADER,
  isMixoraDevServerResponse,
  listenOnDesktopOrigin,
};
