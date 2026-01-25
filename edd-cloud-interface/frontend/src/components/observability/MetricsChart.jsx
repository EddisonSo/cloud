import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Legend,
} from "recharts";

const colors = {
  cpu: "#3b82f6",     // blue
  mem: "#22c55e",     // green
  disk: "#f59e0b",    // amber
};

function formatTimestamp(timestamp) {
  const date = new Date(timestamp * 1000);
  return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

function formatPercent(value) {
  return `${value.toFixed(1)}%`;
}

export function MetricsChart({ data, title, dataKey, color, yAxisLabel = "%" }) {
  if (!data || data.length === 0) {
    return (
      <div className="h-48 flex items-center justify-center text-muted-foreground">
        No data available
      </div>
    );
  }

  return (
    <div className="h-48">
      <h4 className="text-sm font-medium mb-2">{title}</h4>
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={data} margin={{ top: 5, right: 10, left: 0, bottom: 5 }}>
          <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
          <XAxis
            dataKey="t"
            tickFormatter={formatTimestamp}
            className="text-xs"
            tick={{ fill: "currentColor" }}
            stroke="currentColor"
          />
          <YAxis
            domain={[0, 100]}
            tickFormatter={(v) => `${v}%`}
            className="text-xs"
            tick={{ fill: "currentColor" }}
            stroke="currentColor"
            width={45}
          />
          <Tooltip
            contentStyle={{
              backgroundColor: "var(--background)",
              border: "1px solid var(--border)",
              borderRadius: "0.375rem",
            }}
            labelFormatter={formatTimestamp}
            formatter={(value) => [formatPercent(value), dataKey]}
          />
          <Line
            type="monotone"
            dataKey={dataKey}
            stroke={color}
            strokeWidth={2}
            dot={false}
            activeDot={{ r: 4 }}
          />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}

export function MultiMetricsChart({ data, title }) {
  if (!data || data.length === 0) {
    return (
      <div className="h-56 flex items-center justify-center text-muted-foreground">
        No data available
      </div>
    );
  }

  return (
    <div className="h-56">
      <h4 className="text-sm font-medium mb-2">{title}</h4>
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={data} margin={{ top: 5, right: 10, left: 0, bottom: 5 }}>
          <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
          <XAxis
            dataKey="t"
            tickFormatter={formatTimestamp}
            className="text-xs"
            tick={{ fill: "currentColor" }}
            stroke="currentColor"
          />
          <YAxis
            domain={[0, 100]}
            tickFormatter={(v) => `${v}%`}
            className="text-xs"
            tick={{ fill: "currentColor" }}
            stroke="currentColor"
            width={45}
          />
          <Tooltip
            contentStyle={{
              backgroundColor: "var(--background)",
              border: "1px solid var(--border)",
              borderRadius: "0.375rem",
            }}
            labelFormatter={formatTimestamp}
            formatter={(value, name) => [formatPercent(value), name]}
          />
          <Legend />
          <Line
            type="monotone"
            dataKey="cpu"
            name="CPU"
            stroke={colors.cpu}
            strokeWidth={2}
            dot={false}
            activeDot={{ r: 4 }}
          />
          <Line
            type="monotone"
            dataKey="mem"
            name="Memory"
            stroke={colors.mem}
            strokeWidth={2}
            dot={false}
            activeDot={{ r: 4 }}
          />
          <Line
            type="monotone"
            dataKey="disk"
            name="Disk"
            stroke={colors.disk}
            strokeWidth={2}
            dot={false}
            activeDot={{ r: 4 }}
          />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}
