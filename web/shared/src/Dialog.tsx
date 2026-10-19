/** The site's own dialog for a warning or a question, opened as a modal: the page behind is inert, Escape cancels, and focus goes back to what opened it. */
import { useCallback, useEffect, useId, useRef, useState, type SubmitEvent } from 'react';
import { answerOf, firstFocus, hasField, type Focus, type Question, type TextQuestion } from './question';

export type { Question, TextQuestion } from './question';

/** Dialog shows a question until it is answered or cancelled, then calls onDone with the answer, or null for Cancel and Escape. */
function Dialog({ question: q, onDone }: { question: Question | TextQuestion; onDone: (answer: string | null) => void }) {
  const box = useRef<HTMLDialogElement>(null);
  const controls = { field: useRef<HTMLInputElement>(null), cancel: useRef<HTMLButtonElement>(null), action: useRef<HTMLButtonElement>(null) } satisfies Record<Focus, unknown>;
  const answer = useRef<string | null>(null);
  const [text, setText] = useState('');
  const [blank, setBlank] = useState(false);
  const id = useId();
  const focus = firstFocus(q);
  useEffect(() => {
    box.current?.showModal();
    controls[focus].current?.focus();
  }, []);
  const submit = (e: SubmitEvent) => {
    e.preventDefault();
    const a = answerOf(q, text);
    if (a === null) {
      setBlank(true);
      controls.field.current?.focus();
      return;
    }
    answer.current = a;
    box.current?.close();
  };
  return (
    <dialog
      ref={box}
      className="site-dialog"
      role="alertdialog"
      aria-labelledby={`${id}-heading`}
      aria-describedby={q.text === undefined ? undefined : `${id}-text`}
      onClose={() => onDone(answer.current)}
    >
      <form onSubmit={submit} noValidate>
        <h2 id={`${id}-heading`} className="site-dialog-heading">
          {q.heading}
        </h2>
        {q.text !== undefined && (
          <p id={`${id}-text`} className="site-dialog-text">
            {q.text}
          </p>
        )}
        {hasField(q) && (
          <label className="site-dialog-field">
            <span>
              {q.field}
              {!q.required && <span className="site-dialog-optional"> (optional)</span>}
            </span>
            <input
              ref={controls.field}
              className="input-antique"
              value={text}
              required={q.required}
              maxLength={q.maxLength}
              autoComplete="off"
              aria-invalid={blank}
              aria-describedby={blank ? `${id}-blank` : undefined}
              onChange={(e) => setText(e.target.value)}
            />
          </label>
        )}
        {blank && (
          <p id={`${id}-blank`} className="site-dialog-blank" role="alert">
            This cannot be empty.
          </p>
        )}
        <div className="site-dialog-buttons">
          <button ref={controls.cancel} type="button" className="btn-antique" onClick={() => box.current?.close()}>
            Cancel
          </button>
          <button ref={controls.action} type="submit" className={q.final === true ? 'btn-antique btn-danger' : 'btn-antique'}>
            {q.action}
          </button>
        </div>
      </form>
    </dialog>
  );
}

interface Open {
  key: number;
  question: Question | TextQuestion;
  done: (answer: string | null) => void;
}

/**
 * useDialog gives a component its dialog. confirm resolves true once the action is chosen; ask resolves the typed
 * answer, trimmed, or null when cancelled. Render element once, anywhere in the component: it opens above the page.
 */
export function useDialog() {
  const [open, setOpen] = useState<Open | null>(null);
  const opened = useRef(0);
  const show = useCallback((question: Question | TextQuestion, done: Open['done']) => setOpen({ key: ++opened.current, question, done }), []);
  const confirm = useCallback((question: Question) => new Promise<boolean>((resolve) => show(question, (a) => resolve(a !== null))), [show]);
  const ask = useCallback((question: TextQuestion) => new Promise<string | null>((resolve) => show(question, resolve)), [show]);
  const element =
    open === null ? null : (
      <Dialog
        key={open.key}
        question={open.question}
        onDone={(a) => {
          setOpen(null);
          open.done(a);
        }}
      />
    );
  return { confirm, ask, element };
}
