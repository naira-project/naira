
import Adopters from "../components/Adopters";
import { Card, CardContent } from "../components/ui/card";
import { AdoptersType } from "../hooks/useModels";

export default function Stats({ adopters }: { adopters: AdoptersType[] }) {
    return (
        <div>
            <Card>
                <CardContent>
                    <h3>Adapters</h3>
                    <h1><b>{adopters.length}</b></h1>
                    <h3>Direct Integrations</h3>
                </CardContent>
            </Card>
            <div className="p-3"></div>
            <Adopters adopters={adopters} />
        </div>        

    )
}