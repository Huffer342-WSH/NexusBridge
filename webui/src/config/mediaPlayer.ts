/** 定义重新构建 WebUI 时可替换的媒体播放器实现。 */
export const mediaPlayerConfig = {
  video: 'artplayer',
  audio: 'vidstack',
  image: 'native',
} as const satisfies {
  video: 'artplayer' | 'native';
  audio: 'vidstack' | 'native';
  image: 'native';
};
