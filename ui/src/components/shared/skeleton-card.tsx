export function SkeletonCard() {
  return (
    <div className="rounded-md border border-border p-6 shadow-stripe-ambient animate-pulse">
      <div className="h-3 w-24 rounded-sm bg-muted mb-4" />
      <div className="h-6 w-16 rounded-sm bg-muted mb-3" />
      <div className="space-y-2">
        <div className="h-3 w-full rounded-sm bg-muted" />
        <div className="h-3 w-3/4 rounded-sm bg-muted" />
      </div>
    </div>
  )
}

export function SkeletonTable({ rows = 5 }: { rows?: number }) {
  return (
    <div className="rounded-md border border-border animate-pulse">
      <div className="border-b border-border p-3">
        <div className="flex gap-8">
          <div className="h-3 w-32 rounded-sm bg-muted" />
          <div className="h-3 w-20 rounded-sm bg-muted" />
          <div className="h-3 w-24 rounded-sm bg-muted" />
        </div>
      </div>
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="border-b border-border p-3 last:border-b-0">
          <div className="flex gap-8">
            <div className="h-3 w-48 rounded-sm bg-muted" />
            <div className="h-3 w-16 rounded-sm bg-muted" />
            <div className="h-3 w-24 rounded-sm bg-muted" />
          </div>
        </div>
      ))}
    </div>
  )
}

export function SkeletonPage() {
  return (
    <div className="animate-pulse">
      <div className="h-8 w-48 rounded-sm bg-muted mb-2" />
      <div className="h-4 w-72 rounded-sm bg-muted mb-8" />
      <div className="grid grid-cols-1 gap-5 md:grid-cols-2 lg:grid-cols-4">
        <SkeletonCard />
        <SkeletonCard />
        <SkeletonCard />
        <SkeletonCard />
      </div>
    </div>
  )
}
