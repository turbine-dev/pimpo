import { useId } from 'react'

// The Pimpo: a tuxedo cat, black with a white blaze, muzzle and chest.
// One drawing serves the logo and the mascot, in the 512-unit space of the
// app icon.

export const FACE = 'M256 164c-66 0-116 20-142 60-24 36-24 84-6 120 28 56 84 84 148 84s120-28 148-84c18-36 18-84-6-120-26-40-76-60-142-60Z'
export const PATCH = 'M256 214c-7 0-11 10-13 26-3 22-6 36-18 46-18 4-40 12-52 32-14 24-8 54 18 72 22 16 44 22 65 22s43-6 65-22c26-18 32-48 18-72-12-20-34-28-52-32-12-10-15-24-18-46-2-16-6-26-13-26Z'
export const EAR_L = 'M124 250 146 96c2-14 16-19 26-10l92 96Z'
export const EAR_R = 'M388 250 366 96c-2-14-16-19-26-10l-92 96Z'
const WHISKERS = 'M222 338 118 322M222 350 116 358M290 338l104-16M290 350l106 8'

export type Mood = 'idle' | 'sleep' | 'alert' | 'happy' | 'worried' | 'working' | 'yawn'

// Eyes change with the mood; everything else stays the cat.
function Eyes({ mood }: { mood: Mood }) {
  if (mood === 'sleep' || mood === 'yawn') {
    return <path className="pimpo-eyes" d="M172 266q28 18 56 0M284 266q28 18 56 0" fill="none" stroke="#fff" strokeWidth="9" strokeLinecap="round" />
  }
  if (mood === 'happy') {
    return <path className="pimpo-eyes" d="M172 272q28-30 56 0M284 272q28-30 56 0" fill="none" stroke="#fff" strokeWidth="10" strokeLinecap="round" />
  }
  const big = mood === 'alert'
  const small = mood === 'worried'
  const pr = small ? 9 : big ? 16 : 13
  const py = small ? 15 : big ? 24 : 21
  return (
    <g className="pimpo-eyes">
      <ellipse cx="200" cy="262" rx="31" ry="37" fill="#fff" stroke="#111" strokeWidth="7" />
      <ellipse cx="312" cy="262" rx="31" ry="37" fill="#fff" stroke="#111" strokeWidth="7" />
      <g className="pimpo-pupils">
        <ellipse cx="207" cy="266" rx={pr} ry={py} fill="#111" />
        <ellipse cx="305" cy="266" rx={pr} ry={py} fill="#111" />
        <circle cx="212" cy="257" r="5.5" fill="#fff" />
        <circle cx="310" cy="257" r="5.5" fill="#fff" />
      </g>
      {small && <path d="M170 230l50-16M342 230l-50-16" stroke="#fff" strokeWidth="8" strokeLinecap="round" />}
    </g>
  )
}

// Head draws the cat's head; whiskers are left out at small sizes.
export function Head({ mood = 'idle', whiskers = true }: { mood?: Mood; whiskers?: boolean }) {
  const id = useId().replace(/:/g, '')
  return (
    <g>
      <defs>
        <clipPath id={`${id}in`}><path d={PATCH} /></clipPath>
        <clipPath id={`${id}face`}><path d={FACE} /></clipPath>
        <mask id={`${id}out`}><rect width="512" height="512" fill="#fff" /><path d={PATCH} fill="#000" /></mask>
      </defs>
      <g className="pimpo-ears">
        <path className="pimpo-ear-l" d={EAR_L} fill="#111" stroke="#111" strokeWidth="12" strokeLinejoin="round" />
        <path className="pimpo-ear-r" d={EAR_R} fill="#111" stroke="#111" strokeWidth="12" strokeLinejoin="round" />
        <path d="M152 196 162 122l46 50Z" fill="#fff" opacity=".18" />
        <path d="M360 196 350 122l-46 50Z" fill="#fff" opacity=".18" />
      </g>
      <path d={FACE} fill="#111" />
      <path d={PATCH} fill="#fff" />
      <Eyes mood={mood} />
      <path d="M240 318c0-6 7-9 16-9s16 3 16 9c0 7-9 14-16 16-7-2-16-9-16-16Z" fill="#111" />
      {mood === 'yawn'
        ? <g><ellipse cx="256" cy="360" rx="20" ry="28" fill="#111" /><ellipse cx="256" cy="374" rx="11" ry="9" fill="#9a9a9a" /></g>
        : mood === 'alert'
        ? <ellipse cx="256" cy="352" rx="10" ry="12" fill="#111" />
        : <path d={mood === 'worried' ? 'M236 356q20-12 40 0' : 'M256 334v10m0 0c-6 10-18 12-26 6m26-6c6 10 18 12 26 6'} fill="none" stroke="#111" strokeWidth="6" strokeLinecap="round" strokeLinejoin="round" />}
      {whiskers && (
        <g strokeWidth="4.5" strokeLinecap="round" fill="none">
          <path d={WHISKERS} stroke="#111" clipPath={`url(#${id}in)`} />
          <path d={WHISKERS} stroke="#fff" mask={`url(#${id}out)`} clipPath={`url(#${id}face)`} />
        </g>
      )}
    </g>
  )
}

// Logo is the app icon: the head on a white rounded square.
export function Logo({ size = 28 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 512 512" aria-hidden>
      <rect x="4" y="4" width="504" height="504" rx="116" fill="#fff" stroke="#d9d9d9" strokeWidth="8" />
      <Head whiskers={size >= 40} />
    </svg>
  )
}
