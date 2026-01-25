const timeRanges = [
  { value: "1h", label: "1h" },
  { value: "6h", label: "6h" },
  { value: "24h", label: "24h" },
  { value: "7d", label: "7d" },
];

export function TimeRangeSelector({ value, onChange }) {
  return (
    <div className="inline-flex rounded-md border border-border bg-secondary">
      {timeRanges.map((range) => (
        <button
          key={range.value}
          onClick={() => onChange(range.value)}
          className={`px-3 py-1.5 text-sm font-medium transition-colors first:rounded-l-md last:rounded-r-md ${
            value === range.value
              ? "bg-primary text-primary-foreground"
              : "text-muted-foreground hover:text-foreground hover:bg-muted"
          }`}
        >
          {range.label}
        </button>
      ))}
    </div>
  );
}
