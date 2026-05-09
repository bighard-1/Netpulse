import Foundation

struct TrafficSegment: Identifiable {
    let id = UUID()
    let points: [TrafficHistoryPoint]
}

struct TrafficRenderModel {
    let decimated: [TrafficHistoryPoint]
    let inSegments: [TrafficSegment]
    let outSegments: [TrafficSegment]
    let yMax: Double
}

enum ChartDataProcessor {
    static func buildTrafficModel(points: [TrafficHistoryPoint], maxPoints: Int = 2200) async -> TrafficRenderModel {
        await Task.detached(priority: .userInitiated) {
            let clean = points.sorted { $0.timestamp < $1.timestamp }
            let decimated = decimate(clean, maxPoints: maxPoints)
            let inSegments = splitSegments(decimated, inDirection: true)
            let outSegments = splitSegments(decimated, inDirection: false)
            let vmax = max(
                1.0,
                decimated.compactMap { finite($0.traffic_in_bps) }.max() ?? 0,
                decimated.compactMap { finite($0.traffic_out_bps) }.max() ?? 0
            )
            return TrafficRenderModel(decimated: decimated, inSegments: inSegments, outSegments: outSegments, yMax: vmax * 1.1)
        }.value
    }

    static func decimate(_ src: [TrafficHistoryPoint], maxPoints: Int) -> [TrafficHistoryPoint] {
        guard src.count > maxPoints, maxPoints > 0 else { return src }
        let bucket = max(1, src.count / max(1, maxPoints / 2))
        var out: [TrafficHistoryPoint] = []
        out.reserveCapacity(maxPoints)
        var i = 0
        while i < src.count {
            if Task.isCancelled { return out }
            let end = min(src.count, i + bucket)
            let slice = Array(src[i..<end])
            let inVals = slice.compactMap { finite($0.traffic_in_bps) }
            let outVals = slice.compactMap { finite($0.traffic_out_bps) }
            if inVals.isEmpty && outVals.isEmpty {
                out.append(TrafficHistoryPoint(timestamp: slice[slice.count / 2].timestamp, traffic_in_bps: nil, traffic_out_bps: nil))
                i += bucket
                continue
            }
            let mean = TrafficHistoryPoint(
                timestamp: slice[slice.count / 2].timestamp,
                traffic_in_bps: inVals.isEmpty ? nil : inVals.reduce(0, +) / Double(inVals.count),
                traffic_out_bps: outVals.isEmpty ? nil : outVals.reduce(0, +) / Double(outVals.count)
            )
            var peak = slice[0]
            var peakVal = -Double.greatestFiniteMagnitude
            for p in slice {
                let local = max(abs(finite(p.traffic_in_bps) ?? 0), abs(finite(p.traffic_out_bps) ?? 0))
                if local > peakVal {
                    peakVal = local
                    peak = p
                }
            }
            out.append(mean)
            out.append(peak)
            i += bucket
        }
        return out.sorted { $0.timestamp < $1.timestamp }
    }

    static func splitSegments(_ points: [TrafficHistoryPoint], inDirection: Bool) -> [TrafficSegment] {
        var segments: [TrafficSegment] = []
        var cur: [TrafficHistoryPoint] = []
        for p in points {
            let value = inDirection ? finite(p.traffic_in_bps) : finite(p.traffic_out_bps)
            if value == nil {
                if !cur.isEmpty {
                    segments.append(TrafficSegment(points: cur))
                    cur.removeAll(keepingCapacity: true)
                }
                continue
            }
            cur.append(p)
        }
        if !cur.isEmpty {
            segments.append(TrafficSegment(points: cur))
        }
        return segments
    }
}

func finite(_ v: Double?) -> Double? {
    guard let x = v, x.isFinite else { return nil }
    return x
}
