import { Card, CardDescription, CardTitle } from '../components/ui/Card'

/**
 * Stand-in for every route that doesn't have a real screen yet. Each real
 * page replaces its own PlaceholderPage usage when its phase is built —
 * see BUILD_GUIDE.md for which phase owns which route.
 */
export function PlaceholderPage({ title, phase }: { title: string; phase: string }) {
  return (
    <main className="mx-auto max-w-2xl px-8 py-16">
      <Card>
        <CardTitle>{title}</CardTitle>
        <CardDescription className="mt-2">
          This screen is not built yet — it's wired into routing as a placeholder so navigation
          works end to end. Real content lands in {phase}.
        </CardDescription>
      </Card>
    </main>
  )
}
