#!/usr/bin/env python

import argparse
import datetime
from sqlalchemy import create_engine
from sqlalchemy import Column, DateTime, String
from sqlalchemy.ext.declarative import declarative_base
from sqlalchemy.orm import sessionmaker

base = declarative_base()

class ReleaseTags(base):
    __tablename__ = 'release_tags'

    id = Column(String, primary_key=True)
    release_tag = Column(String)
    release = Column(String)
    release_time = Column(DateTime)
    stream = Column(String)
    phase = Column(String)
    reject_reason = Column(String)

def selectReleases(session, release, stream, showAll, days):
    selectedTags = []
    start = datetime.datetime.utcnow() - datetime.timedelta(days=days)
    releaseTags = session.query(ReleaseTags).filter(ReleaseTags.phase == "Rejected", ReleaseTags.release_time >= start).all()
    for releaseTag in releaseTags:
        if release and releaseTag.release != release:
            continue
        if stream and releaseTag.stream != stream:
            continue
        if not showAll and releaseTag.reject_reason:
            continue
        selectedTags.append(releaseTag)

    return selectedTags

def printReleases(selectedTags):
    print("%-10s%-50s%-20s%-20s" % ("index", "release tag", "phase", "reject reason"))
    for idx, releaseTag in enumerate(selectedTags):
        print("%-10d%-50s%-20s%-20s" % (idx+1, releaseTag.release_tag, releaseTag.phase, releaseTag.reject_reason))

def list(session, release, stream, showAll, days):
    selectedTags = selectReleases(session, release, stream, showAll, days)
    printReleases(selectedTags)

def categorizeSingle(session, releaseTag):
    reject_reasons = ["TEST_FLAKE", "CLOUD_INFRA", "CLOUD_QUOTA", "RH_INFRA", "PRODUCT_REGRESSION", "TEST_REGRESSION"]
    releaseTags = session.query(ReleaseTags).filter(ReleaseTags.release_tag == releaseTag).all()
    for releaseTag in releaseTags:
        print("Please choose the reject reason for tag %s from the following list:" % releaseTag.release_tag)
        for idx, reason in enumerate(reject_reasons):
            print("%10d: %20s" % (idx+1, reason))

        while True:
            val = input("Enter your selection between 1 and " + str(len(reject_reasons)) + ": ")
            try:
                index = int(val)
                if index > 0 and index <= len(reject_reasons):
                    break
            except ValueError:
                continue
        releaseTag.reject_reason = reject_reasons[index-1]
    session.commit()

def categorize(session, release, stream, showAll, days):
    selectedTags = selectReleases(session, release, stream, showAll, days)
    while True:
        if len(selectedTags) == 0:
            print("No payloads are available to select, exiting.")
            break
        printReleases(selectedTags)
        val = input("Select tag between 1 and " + str(len(selectedTags)) + " to categorize, enter q to exit: ")
        if val == "q":
            break
        try:
            index = int(val)
            if index > 0 and index <= len(selectedTags):
                categorizeSingle(session, selectedTags[index-1].release_tag)
        except ValueError:
            continue
    session.commit()

def verifyArgs(args):
    if args["days"] < 1:
        print("Please enter a positive number for days.")
        return False
    return True

if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='View or Update payload reject reasons to DB')
    parser.add_argument('-d', '--dsn', help='Specifies the DSN used to connect to DB', default="postgresql://postgres:@localhost:5432/postgres")
    parser.add_argument("--days", type=int, help="The number of days to query for", default=14)
    subparsers = parser.add_subparsers(title='subcommands', description='valid subcommands', help='Supported operations', required=True)
    list_parser = subparsers.add_parser('list', help='list rejected payloads')
    list_parser.set_defaults(action='list')
    list_parser.add_argument('-r', '--release', help='Specifies a release, like 4.11', default=None)
    list_parser.add_argument('-s', '--stream', help='Specifies a stream, like nightly or ci', default=None)
    list_parser.add_argument('-a', '--all', help='List all rejected payloads. If not specified , list only uncategorized ones.', action='store_true')

    categorize_parser = subparsers.add_parser('categorize', help='categorize a rejected payload')
    categorize_parser.set_defaults(action='categorize')
    categorize_parser.add_argument('-t', '--release_tag', help='Specifies a release payload tag, like 4.11.0-0.nightly-2022-06-25-081133', default=None)
    categorize_parser.add_argument('-r', '--release', help='Specifies a release, like 4.11', default=None)
    categorize_parser.add_argument('-s', '--stream', help='Specifies a stream, like nightly or ci', default=None)
    categorize_parser.add_argument('-a', '--all', help='List all rejected payloads. If not specified , list only uncategorized ones.', action='store_true')

    args = vars(parser.parse_args())

    if verifyArgs(args) == False:
        exit(1)

    db = create_engine(args["dsn"])

    Session = sessionmaker(db)
    session = Session()

    base.metadata.create_all(db)

    if args["action"] == "categorize":
        if args["release_tag"]:
            categorizeSingle(session, args["release_tag"])
        else:
            categorize(session, args["release"], args["stream"], args["all"], args["days"])
    else:
        list(session, args["release"], args["stream"], args["all"], args["days"])

