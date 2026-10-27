// Agent CLIs run inside the project container, whose clock is UTC, so a usage
// limit error arrives as "You've hit your session limit · resets 7:50pm (UTC)".
// This rewrites that reset time in the viewer's own time zone. Text that does
// not match the pattern is returned unchanged.

/** When the message was sent, and where it is being read. */
export interface LimitResetClock {
  /** The message's own timestamp: the reset it names is the next one after it. */
  sentMs: number;
  nowMs: number;
  /** The browser's own locale and time zone unless a caller, such as a test, pins them. */
  locale?: string;
  timeZone?: string;
}

const MONTHS = ["jan", "feb", "mar", "apr", "may", "jun", "jul", "aug", "sep", "oct", "nov", "dec"];
const DAY_MS = 24 * 60 * 60 * 1000;

// "resets 7:50pm (UTC)", "resets 7pm (UTC)", "resets Oct 3, 5pm (UTC)" and
// "resets Oct 3 at 5:30pm (UTC)".
const RESET_IN_UTC =
  /\bresets\s+(?:([a-z]{3})[a-z]*\.?\s+(\d{1,2}),?\s+(?:at\s+)?)?(\d{1,2})(?::(\d{2}))?\s*([ap]m)\s*\((?:UTC|Etc\/UTC|GMT)\)/gi;

class LimitResetText {
  localize(message: string, clock: LimitResetClock): string {
    return message.replace(
      RESET_IN_UTC,
      (match, month: string | undefined, day: string | undefined, hour: string, minute: string | undefined, meridiem: string) => {
        const resetMs = this.#resetMs(clock.sentMs, month, day, Number(hour), Number(minute ?? 0), meridiem);
        return resetMs === null ? match : `resets ${this.#format(resetMs, clock)}`;
      },
    );
  }

  /** The instant the text names, taking the first matching one after the message was sent. */
  #resetMs(
    sentMs: number,
    month: string | undefined,
    day: string | undefined,
    hour12: number,
    minute: number,
    meridiem: string,
  ): number | null {
    if (hour12 < 1 || hour12 > 12 || minute > 59) return null;
    const hour = (hour12 % 12) + (meridiem.toLowerCase() === "pm" ? 12 : 0);
    const sent = new Date(sentMs);

    if (month === undefined || day === undefined) {
      let reset = Date.UTC(sent.getUTCFullYear(), sent.getUTCMonth(), sent.getUTCDate(), hour, minute);
      if (reset <= sentMs) reset += DAY_MS;
      return reset;
    }

    const monthIndex = MONTHS.indexOf(month.slice(0, 3).toLowerCase());
    const dayOfMonth = Number(day);
    if (monthIndex < 0) return null;
    for (const year of [sent.getUTCFullYear(), sent.getUTCFullYear() + 1]) {
      const reset = Date.UTC(year, monthIndex, dayOfMonth, hour, minute);
      // Reject dates that roll over, such as "Feb 31".
      if (new Date(reset).getUTCDate() !== dayOfMonth) return null;
      if (reset > sentMs - DAY_MS) return reset;
    }
    return null;
  }

  /** The time alone when it falls today for the viewer, otherwise the date as well, with the zone. */
  #format(resetMs: number, clock: LimitResetClock): string {
    const reset = new Date(resetMs);
    const zone = { timeZone: clock.timeZone } as const;
    const day = (date: Date) => date.toLocaleDateString("en-CA", zone);
    const sameDay = day(reset) === day(new Date(clock.nowMs));
    const text = reset.toLocaleString(clock.locale, {
      ...zone,
      hour: "numeric",
      minute: "2-digit",
      ...(sameDay ? {} : { month: "short", day: "numeric" }),
    });
    const zoneName = new Intl.DateTimeFormat(clock.locale, { ...zone, timeZoneName: "short" })
      .formatToParts(reset)
      .find((part) => part.type === "timeZoneName")?.value;
    return zoneName ? `${text} (${zoneName})` : text;
  }
}

export const limitResetText = new LimitResetText();
