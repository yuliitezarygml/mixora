const contentSecurityPolicy =
  "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' https://*.sndcdn.com https://*.scdn.co https://*.ytimg.com https://*.ggpht.com https://*.bcbits.com https://*.userapi.com https://*.vkuseraudio.net https://*.okcdn.ru data:; media-src 'self' https://*.sndcdn.com https://playback.media-streaming.soundcloud.cloud https://*.scdn.co blob:; connect-src 'self' https://*.sndcdn.com https://playback.media-streaming.soundcloud.cloud https://*.scdn.co; worker-src 'self' blob:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'";

module.exports = { contentSecurityPolicy };
