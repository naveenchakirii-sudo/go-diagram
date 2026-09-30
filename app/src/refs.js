// Keys used to look up rendered struct/field DOM nodes when drawing edges.
export const structKey = (pkg, file, name) => `${pkg}\u0000${file}\u0000${name}`;
export const fieldKey = (pkg, file, name, field) => `${structKey(pkg, file, name)}#${field}`;

export const basename = (path) => path.split(/[\\/]/).pop();

// Curved path from the right edge of a field row to the left edge of the
// target struct's header (or a loop for self-references).
export function edgePath(from, to) {
  const x1 = from.right;
  const y1 = from.top + from.height / 2;
  const x2 = to.left;
  const y2 = to.top + 14;
  if (Math.abs(x2 - x1) < 1 && Math.abs(y2 - y1) < 1) return '';
  if (to.left < from.right && to.right > from.left && to.top <= from.top && to.bottom >= from.bottom) {
    // Self-reference: loop out to the right and back into the header.
    const out = 40;
    return `M ${x1} ${y1} C ${x1 + out} ${y1}, ${to.right + out} ${to.top - out}, ${to.right - 20} ${to.top}`;
  }
  const dx = Math.max(60, Math.abs(x2 - x1) / 2);
  return `M ${x1} ${y1} C ${x1 + dx} ${y1}, ${x2 - dx} ${y2}, ${x2} ${y2}`;
}
