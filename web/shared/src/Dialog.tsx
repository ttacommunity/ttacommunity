/** The site's own dialog for a warning or a question, opened as a modal: the page behind is inert, Escape cancels, and focus goes back to what opened it. */
import { useCallback, useEffect, useId, useRef, useState, type RefObject, type SubmitEvent } from 'react';
import { useModal, type Opener } from './modal';
import { answerOf, firstFocus, hasField, type Focus, type Question, type TextQuestion } from './question';

export type { Question, TextQuestion } from './question';

type Answer = (answer: string | null) => void;

/** Dialog shows a question in box until it is answered or cancelled, then calls onDone with the answer, or null for Cancel and Escape. */
function Dialog({ question: q, opener, box, onDone }: { question: Question | TextQuestion; opener: Opener | null; box: RefObject<HTMLDialogElement | null>; onDone: Answer }) {
  const controls = { field: useRef<HTMLInputElement>(null), cancel: useRef<HTMLButtonElement>(null), action: useRef<HTMLButtonElement>(null) } satisfies Record<Focus, unknown>;
  const answer = useRef<string | null>(null);
  const [text, setText] = useState('');
  const [blank, setBlank] = useState(false);
  const id = useId();
  const focus = firstFocus(q);
  const closed = useModal(box, opener, (e) => {
    e.stopPropagation();
    onDone(answer.current);
  });
  useEffect(() => controls[focus].current?.focus(), []);
  // Its events stop here: React would pass them up to a form or dialog the element is rendered in.
  const submit = (e: SubmitEvent) => {
    e.preventDefault();
    e.stopPropagation();
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
      onCancel={(e) => e.stopPropagation()}
      onClose={closed}
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
              onChange={(e) => {
                setText(e.target.value);
                setBlank(false);
              }}
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

/**
 * useDialog gives a component its dialog. confirm resolves true once the action is chosen; ask resolves the typed
 * answer, trimmed, or null when cancelled. opener, the button clicked, has the focus back as it closes. Render element
 * once, anywhere in the component: it opens above the page. A question not answered (element not rendered, another
 * question asked, the component gone) is answered as cancelled.
 */
export function useDialog() {
  const [open, setOpen] = useState<{ key: number; question: Question | TextQuestion; opener: Opener | null } | null>(null);
  const opened = useRef(0);
  const box = useRef<HTMLDialogElement>(null);
  const waiting = useRef<Answer | null>(null);
  const answer = useCallback<Answer>((a) => {
    const done = waiting.current;
    waiting.current = null;
    setOpen(null);
    done?.(a);
  }, []);
  const show = useCallback((question: Question | TextQuestion, opener: Opener | null, done: Answer) => {
    waiting.current?.(null);
    waiting.current = done;
    setOpen({ key: ++opened.current, question, opener });
  }, []);
  useEffect(() => {
    if (open === null || box.current !== null) return;
    console.error('useDialog: element is not rendered, so the question is answered as cancelled');
    answer(null);
  }, [open, answer]);
  useEffect(() => () => waiting.current?.(null), []);
  const confirm = useCallback(
    (question: Question, opener: Opener | null) => new Promise<boolean>((resolve) => show(question, opener, (a) => resolve(a !== null))),
    [show],
  );
  const ask = useCallback((question: TextQuestion, opener: Opener | null) => new Promise<string | null>((resolve) => show(question, opener, resolve)), [show]);
  const element = open === null ? null : <Dialog key={open.key} question={open.question} opener={open.opener} box={box} onDone={answer} />;
  return { confirm, ask, element };
}
