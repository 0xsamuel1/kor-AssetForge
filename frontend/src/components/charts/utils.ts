/**
 * Largest Triangle Three Buckets (LTTB) data decimation.
 * Keeps the visually significant points for large time-series datasets.
 */
export function largestTriangleThreeBuckets<T extends Record<string, unknown>>(
  data: T[],
  threshold: number,
  yKey: string
): T[] {
  if (data.length <= threshold) return data;

  const sampled: T[] = [];
  const bucketSize = (data.length - 2) / (threshold - 2);

  sampled.push(data[0]);

  for (let i = 0; i < threshold - 2; i++) {
    const bucketStart = Math.floor((i + 1) * bucketSize) + 1;
    const bucketEnd = Math.min(
      Math.floor((i + 2) * bucketSize) + 1,
      data.length
    );

    const avgX =
      Array.from({ length: bucketEnd - bucketStart }, (_, k) => bucketStart + k).reduce(
        (s, v) => s + v,
        0
      ) /
      (bucketEnd - bucketStart);
    const avgY =
      data.slice(bucketStart, bucketEnd).reduce(
        (s, d) => s + (Number(d[yKey]) || 0),
        0
      ) /
      (bucketEnd - bucketStart);

    const rangeStart = Math.floor(i * bucketSize) + 1;
    const rangeEnd = Math.min(Math.floor((i + 1) * bucketSize) + 1, data.length);

    const pointA = sampled[sampled.length - 1];
    const aX = rangeStart;
    const aY = Number(pointA[yKey]) || 0;

    let maxArea = -1;
    let maxPoint = data[rangeStart];

    for (let j = rangeStart; j < rangeEnd; j++) {
      const area = Math.abs(
        (aX - avgX) * ((Number(data[j][yKey]) || 0) - aY) -
          (aX - j) * (avgY - aY)
      );
      if (area > maxArea) {
        maxArea = area;
        maxPoint = data[j];
      }
    }

    sampled.push(maxPoint);
  }

  sampled.push(data[data.length - 1]);
  return sampled;
}
