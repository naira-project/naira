const LINES = ['More models.', 'More data.', 'More possibilities.'];

export function WelcomeBannerTagline() {
  return (
    <div className="flex flex-1 flex-col items-end justify-center gap-[9px] self-stretch py-5 pr-5 pl-10">
      {LINES.map((line) => (
        <p
          key={line}
          className="self-stretch text-xl leading-normal font-medium text-deep-sapphire"
        >
          {line}
        </p>
      ))}
    </div>
  );
}
