import { Card, CardContent } from '../components/ui/card';
import { AdoptersType } from '../hooks/useModels';

export default function Adopters({ adopters }: { adopters: AdoptersType[] }) {
    if (adopters.length === 0) {
        return <p className="text-sm text-muted-foreground">No adopters found.</p>;
    }

    return (
        <div className="flex flex-col gap-3">
            <Card>
                <CardContent>
                    <h3 className='p-3'>Adapters - apps and services using this model directly</h3>
            <div className='flex flex-col gap-2'>
            {adopters.map((adopter, i) => (
                    <Card key={i}>
                        <CardContent className="flex flex-col gap-1">
                            <p className="text-sm font-semibold">{adopter.name}</p>
                            <p className="text-xs text-muted-foreground">Namespace: {adopter.namespace}</p>
                            <p className="text-xs text-muted-foreground">Team: {adopter.team}</p>
                        </CardContent>
                    </Card>
            ))}
            </div>
                </CardContent>
            </Card>
        </div>
    );
}
