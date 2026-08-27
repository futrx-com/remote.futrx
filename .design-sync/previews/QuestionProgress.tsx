// QuestionProgress — "n / total" counter plus per-question dots; lives in the
// blue "Agent is asking" header strip, so previews recreate that strip.
import { QuestionProgress } from "remote.futrx-web";

const questions = [
  { question: "How should we roll out the service worker?", options: [] },
  { question: "Which browsers must the first release support?", options: [] },
  { question: "Where should delivery failures be reported?", options: [] },
  { question: "Who signs off on the rollout?", options: [] },
];

const Strip = ({ children }: { children?: any }) => (
  <div className="w-full max-w-xl">
    <div
      className="rounded-lg border border-accent-blue/40 bg-accent-blue/10 px-3 py-2
                 text-[11px] text-accent-blue flex items-center justify-between gap-2"
    >
      <span>Agent is asking</span>
      {children}
    </div>
  </div>
);

export const FirstQuestion = () => (
  <Strip>
    <QuestionProgress
      questions={questions}
      page={0}
      total={questions.length}
      questionAnswered={() => false}
    />
  </Strip>
);

export const MidwayAnswered = () => (
  <Strip>
    <QuestionProgress
      questions={questions}
      page={2}
      total={questions.length}
      questionAnswered={(index: number) => index < 2}
    />
  </Strip>
);
