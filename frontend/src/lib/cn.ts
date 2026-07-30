import { clsx, type ClassValue } from 'clsx'

/** Joins conditional class names, following the shadcn/ui convention this
 * project's component set is styled to match
 * (documentation/08-frontend-architecture.md, "shadcn/ui"). */
export function cn(...inputs: ClassValue[]): string {
  return clsx(inputs)
}
