/** 通用格式化模块集中维护媒体页面使用的显示规则。 */

const byteUnits = ['B', 'KB', 'MB', 'GB', 'TB'] as const;

/** 将字节数格式化为易读大小。 */
export function formatByteSize(bytes?: number): string {
	if (!bytes || bytes <= 0) return '-';
	let value = bytes;
	let unitIndex = 0;
	while (value >= 1024 && unitIndex < byteUnits.length - 1) {
		value /= 1024;
		unitIndex += 1;
	}
	return `${value.toFixed(value >= 100 ? 0 : 1)} ${byteUnits[unitIndex]}`;
}

/** 将每秒字节数格式化为传输速度。 */
export function formatByteSpeed(bytesPerSecond?: number): string {
	return bytesPerSecond && bytesPerSecond > 0 ? `${formatByteSize(bytesPerSecond)}/s` : '0 B/s';
}
