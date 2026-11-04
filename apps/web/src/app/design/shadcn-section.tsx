'use client';

import { ChevronDown, Plus } from 'lucide-react';

import { Badge } from '../../components/ui/badge';
import { Button } from '../../components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '../../components/ui/card';
import { CheckboxControl } from '../../components/ui/checkbox';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '../../components/ui/dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '../../components/ui/dropdown-menu';
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '../../components/ui/empty';
import { Input } from '../../components/ui/input';
import { Label } from '../../components/ui/label';
import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from '../../components/ui/pagination';
import {
  RadioGroupControl,
  RadioGroupItem,
} from '../../components/ui/radio-group';
import { Separator } from '../../components/ui/separator';
import {
  Select as SelectMenu,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../../components/ui/select-menu';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '../../components/ui/sheet';
import {
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '../../components/ui/table';
import { TextareaControl } from '../../components/ui/textarea-control';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '../../components/ui/tooltip';
import { Panel } from './components-section';

export function ShadcnSection() {
  return (
    <section aria-labelledby="shadcn" className="flex flex-col gap-8">
      <div className="flex flex-col gap-2">
        <h2 id="shadcn" className="font-display text-h2">
          shadcn primitives
        </h2>
        <p className="max-w-prose text-body text-text-muted">
          Radix behaviour with SwiftDrop tokens. These are the foundation pieces
          screens compose from; they are not a second visual language.
        </p>
      </div>

      <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
        <Panel title="Card and badge">
          <Card>
            <CardHeader>
              <CardTitle>Colombo to Kandy</CardTitle>
              <CardDescription>
                Standard parcel, 2.4 kg, collected today.
              </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-wrap items-center gap-2">
              <Badge>In transit</Badge>
              <Badge variant="secondary">Standard</Badge>
              <Badge variant="outline">Insured</Badge>
              <Badge variant="destructive">Failed</Badge>
            </CardContent>
            <CardFooter className="justify-between border-t border-line pt-4">
              <span className="text-caption text-text-muted">SDK7M2Q9XP4</span>
              <Button size="sm" variant="secondary">
                View
              </Button>
            </CardFooter>
          </Card>
        </Panel>

        <Panel title="Form controls">
          <div className="flex flex-col gap-4">
            <div className="flex flex-col gap-2">
              <Label htmlFor="demo-shadcn-name">Recipient</Label>
              <Input id="demo-shadcn-name" placeholder="Nimali Perera" />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="demo-shadcn-note">Delivery note</Label>
              <TextareaControl
                id="demo-shadcn-note"
                placeholder="Leave at the front desk."
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="demo-shadcn-status">Status</Label>
              <SelectMenu defaultValue="SENT">
                <SelectTrigger id="demo-shadcn-status" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="PENDING">PENDING</SelectItem>
                  <SelectItem value="SENT">SENT</SelectItem>
                  <SelectItem value="FAILED">FAILED</SelectItem>
                </SelectContent>
              </SelectMenu>
            </div>
            <div className="flex items-center gap-2">
              <CheckboxControl id="demo-shadcn-insured" defaultChecked />
              <Label htmlFor="demo-shadcn-insured">Insure this parcel</Label>
            </div>
            <RadioGroupControl defaultValue="standard" className="gap-2">
              <div className="flex items-center gap-2">
                <RadioGroupItem id="demo-standard" value="standard" />
                <Label htmlFor="demo-standard">Standard</Label>
              </div>
              <div className="flex items-center gap-2">
                <RadioGroupItem id="demo-express" value="express" />
                <Label htmlFor="demo-express">Express</Label>
              </div>
            </RadioGroupControl>
          </div>
        </Panel>

        <Panel title="Dialog, sheet and menu">
          <div className="flex flex-wrap gap-3">
            <Dialog>
              <DialogTrigger asChild>
                <Button variant="secondary">Open dialog</Button>
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>Cancel this delivery?</DialogTitle>
                  <DialogDescription>
                    The parcel returns to the sender and the tracking history
                    keeps the cancellation.
                  </DialogDescription>
                </DialogHeader>
              </DialogContent>
            </Dialog>

            <Sheet>
              <SheetTrigger asChild>
                <Button variant="secondary">Open sheet</Button>
              </SheetTrigger>
              <SheetContent side="right">
                <SheetHeader>
                  <SheetTitle>Delivery details</SheetTitle>
                  <SheetDescription>
                    A sheet is the drawer pattern on a wider screen.
                  </SheetDescription>
                </SheetHeader>
              </SheetContent>
            </Sheet>

            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="secondary">
                  Actions
                  <ChevronDown size={16} strokeWidth={1.5} />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start">
                <DropdownMenuLabel>Parcel</DropdownMenuLabel>
                <DropdownMenuSeparator />
                <DropdownMenuItem>View tracking</DropdownMenuItem>
                <DropdownMenuItem disabled>Archive parcel</DropdownMenuItem>
                <DropdownMenuItem>Reassign courier</DropdownMenuItem>
                <DropdownMenuItem variant="destructive">
                  Cancel delivery
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>

            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button variant="ghost" aria-label="Add a parcel">
                    <Plus size={16} strokeWidth={1.5} />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Add a parcel</TooltipContent>
              </Tooltip>
            </TooltipProvider>
          </div>
          <Separator />
          <p className="text-body-sm text-text-muted">
            Radix supplies focus trapping, Escape handling and menu keyboard
            navigation.
          </p>
        </Panel>

        <Panel title="Table and pagination">
          <Table>
            <TableCaption>Recent delivery attempts.</TableCaption>
            <TableHeader>
              <TableRow>
                <TableHead>Recipient</TableHead>
                <TableHead>Status</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow>
                <TableCell className="font-mono">n***@example.com</TableCell>
                <TableCell>
                  <Badge variant="outline">SENT</Badge>
                </TableCell>
              </TableRow>
              <TableRow>
                <TableCell className="font-mono">k***@example.com</TableCell>
                <TableCell>
                  <Badge variant="destructive">FAILED</Badge>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
          <Pagination>
            <PaginationContent>
              <PaginationItem>
                <PaginationPrevious disabled />
              </PaginationItem>
              <PaginationItem>
                <PaginationLink isActive>1</PaginationLink>
              </PaginationItem>
              <PaginationItem>
                <PaginationLink>2</PaginationLink>
              </PaginationItem>
              <PaginationItem>
                <PaginationEllipsis />
              </PaginationItem>
              <PaginationItem>
                <PaginationNext />
              </PaginationItem>
            </PaginationContent>
          </Pagination>
        </Panel>
      </div>

      <Panel title="Empty">
        <Empty>
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <Plus size={20} strokeWidth={1.5} />
            </EmptyMedia>
            <EmptyTitle>No saved filters</EmptyTitle>
            <EmptyDescription>
              Save a filter to find the same group of parcels again.
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      </Panel>
    </section>
  );
}
