/**
 * Shown as the empty page, after respective plugins for a viewpoint have been 
 * run. If the page does not consist of any data for the respective user, then this state comes up.
 */
export default function EmptyState() {

  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-[10px] self-stretch pt-10 pb-[220px]">
      <div className="flex h-[262.381px] w-[280.603px] items-center justify-center pt-[31.925px] pr-[35.496px] pb-[30.226px] pl-[33.65px]">
        <img
          src="/empty.svg"
          alt="Ghost image for showing the empty state"
          className="h-full w-full object-contain"
        />
      </div>

      <div className="flex w-[331px] flex-col items-center gap-[24px]">
        <h3 className="text-center font-sans text-2xl font-semibold leading-6 text-[var(--Side-Bar-Text,#C2C0B6)]">
          This Page is Empty
        </h3>
        <p className="text-center font-sans text-xl font-medium leading-6 text-[var(--Side-Bar-Text,#C2C0B6)]">
          There are no data that could be fetched from the plugins.
        </p>
      </div>
    </div>
  );
}
