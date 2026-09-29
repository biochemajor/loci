# Countdown (`countdown.html`) — follow-ups

Noted 2026-09-29 after reverting commit 72bf457 ("Render aurora and confetti at
device pixel density; smooth moon glow"), which made the aurora lose too much
color and glow. That commit is still in the history and can be mined for parts.

## To circle back on

1. **Sharper confetti on high-resolution screens** (low risk)
   - Re-apply just the confetti part of 72bf457: size the `#confetti` canvas to
     `devicePixelRatio` and use `innerWidth`/`innerHeight` for piece positions.
   - Doesn't touch the aurora.

2. **Moon glow tiles at the finish** (verify first)
   - At zero the moon grows (`scale: 1.5`) with wide blurred `box-shadow`s. In
     headless (software-rendered) Chrome this showed faint square tile edges.
   - Check in a normal Chrome window first (`#finish` preview). If the tiles
     show there too, re-apply the radial-gradient glow from 72bf457.

3. **Sharper aurora that keeps the brilliant colors** (needs side-by-side)
   - Rendering at device pixel density made rays thinner, so less colored
     light reached the screen and it looked duller.
   - Try: device-pixel-density rays, plus higher brightness to compensate
     (raise the aurora `strength`, soften the ray mask's floor so less light
     is cut away, and/or keep the glow at `GLOW_RES = 6`).
   - Compare screenshots against the current version at 1x, 2x and 3x before
     shipping; only ship if it is at least as bright and colorful.
