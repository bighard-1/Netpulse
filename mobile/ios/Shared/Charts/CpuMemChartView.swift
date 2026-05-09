import SwiftUI
import Charts

private enum CpuMemSeries: String, CaseIterable {
    case cpu = "CPU(%)"
    case mem = "内存(%)"
}

private struct CpuMemChartPoint: Identifiable {
    let id = UUID()
    let ts: Date
    let series: CpuMemSeries
    let value: Double
}

struct CpuMemChartView: View {
    let cpu: [DeviceHistoryPoint]
    let mem: [DeviceHistoryPoint]
    let showCPU: Bool
    let showMem: Bool

    private var points: [CpuMemChartPoint] {
        var out: [CpuMemChartPoint] = []
        if showCPU {
            out.append(contentsOf: cpu.compactMap { p in
                guard let v = finite(p.cpu_usage) else { return nil }
                return CpuMemChartPoint(ts: p.timestamp, series: .cpu, value: v)
            })
        }
        if showMem {
            out.append(contentsOf: mem.compactMap { p in
                guard let v = finite(p.mem_usage) else { return nil }
                return CpuMemChartPoint(ts: p.timestamp, series: .mem, value: v)
            })
        }
        return out.sorted { $0.ts < $1.ts }
    }

    var body: some View {
        Chart(points) { p in
            LineMark(
                x: .value("时间", p.ts),
                y: .value("值", p.value)
            )
            .foregroundStyle(by: .value("序列", p.series.rawValue))
            .lineStyle(by: .value("序列", p.series.rawValue))
            .interpolationMethod(.linear)
        }
        .chartForegroundStyleScale([
            CpuMemSeries.cpu.rawValue: Color.orange,
            CpuMemSeries.mem.rawValue: Color.cyan
        ])
        .chartLineStyleScale([
            CpuMemSeries.cpu.rawValue: StrokeStyle(lineWidth: 2, dash: [7, 4]),
            CpuMemSeries.mem.rawValue: StrokeStyle(lineWidth: 2)
        ])
        .chartYScale(domain: 0...100)
        .chartYAxis {
            AxisMarks(position: .leading, values: [0, 25, 50, 75, 100]) { value in
                AxisGridLine(); AxisTick()
                AxisValueLabel {
                    if let v = value.as(Double.self) { Text("\(Int(v))%") }
                }
            }
        }
    }
}
